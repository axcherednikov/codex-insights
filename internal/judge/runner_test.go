package judge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCheckCLIReportsMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := CheckCLI(); !errors.Is(err, ErrCLINotFound) {
		t.Fatalf("CheckCLI() error = %v, want ErrCLINotFound", err)
	}
}

func TestRunnerUsesValidatedEphemeralCommandAndReadsResult(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "args.txt")
	installFakeCodex(t, `printf '%s\n' "$@" > "$CODEX_CAPTURE"
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		shift
		printf '%s' '{"ok":true}' > "$1"
		exit 0
	fi
	shift
done
exit 7
`)
	t.Setenv("CODEX_CAPTURE", capture)
	runner := Runner{Model: "judge-model", Effort: "high"}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := runner.Run("private prompt", map[string]string{"type": "object"}, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("Judge result = %+v", result)
	}

	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(args) != 14 {
		t.Fatalf("captured command arguments = %#v", args)
	}
	wantPrefix := []string{
		"exec", "--ephemeral", "-m", "judge-model", "-c", `model_reasoning_effort="high"`,
		"-s", "read-only", "--skip-git-repo-check", "--output-schema",
	}
	if !reflect.DeepEqual(args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("command prefix = %#v, want %#v", args[:len(wantPrefix)], wantPrefix)
	}
	if filepath.Base(args[10]) != judgeSchemaFile || !filepath.IsAbs(args[10]) {
		t.Fatalf("schema path = %q", args[10])
	}
	if args[11] != "-o" {
		t.Fatalf("output flag position = %q", args[11])
	}
	if len(args) < 14 || filepath.Base(args[12]) != judgeResultFile || !filepath.IsAbs(args[12]) || args[13] != "-" {
		t.Fatalf("output path/terminator = %#v", args[12:])
	}
}

func TestRunnerRejectsUnsafeModelAndEffort(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, test := range []struct {
		name   string
		runner Runner
		want   error
	}{
		{name: "model flag", runner: Runner{Model: "--help", Effort: "high"}, want: errInvalidModel},
		{name: "model whitespace", runner: Runner{Model: "model name", Effort: "high"}, want: errInvalidModel},
		{name: "unsupported effort", runner: Runner{Model: "model", Effort: "medium\" --help"}, want: errInvalidEffort},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.runner.Run("", nil, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("Run error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRunnerRunContextCancelsSyntheticCommand(t *testing.T) {
	installFakeCodex(t, "exec sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := (Runner{Model: "judge-model", Effort: "medium"}).RunContext(
		ctx,
		"private prompt",
		map[string]string{"type": "object"},
		new(any),
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunContext error = %v, want context deadline", err)
	}
}

func installFakeCodex(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("synthetic shell executable requires a Unix-like host")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	pathValue := dir
	if currentPath := os.Getenv("PATH"); currentPath != "" {
		pathValue += string(os.PathListSeparator) + currentPath
	}
	t.Setenv("PATH", pathValue)
}

func TestSanitizeStderrKeepsCompactProviderDiagnostic(t *testing.T) {
	secret := "TOP_SECRET_CONVERSATION_FRAGMENT"
	diagnostic := sanitizeStderrForPrompt("ERROR: provider rate limit (429) "+secret, secret)
	if !strings.Contains(diagnostic, "provider rate limit (429)") {
		t.Fatalf("diagnostic = %q", diagnostic)
	}
	if strings.Contains(diagnostic, secret) {
		t.Fatalf("secret stderr content leaked: %q", diagnostic)
	}
}

func TestSanitizeStderrOmitsUnstructuredOutput(t *testing.T) {
	if got := sanitizeStderr("ordinary output\nfull private prompt"); got != privateStderrMessage {
		t.Fatalf("sanitized stderr = %q", got)
	}
}

func TestSanitizeStderrParsesMultilineStructuredError(t *testing.T) {
	secret := "TOP_SECRET_CONVERSATION_FRAGMENT"
	stderr := secret + "\nERROR: {\n  \"type\": \"error\",\n  \"error\": {\n    \"type\": \"invalid_request_error\",\n    \"code\": \"invalid_json_schema\",\n    \"message\": \"uniqueItems is not permitted\"\n  },\n  \"status\": 400\n}\n"
	diagnostic := sanitizeStderrForPrompt(stderr, secret)
	if strings.Contains(diagnostic, secret) {
		t.Fatalf("secret stderr content leaked: %q", diagnostic)
	}
	if !strings.Contains(diagnostic, "invalid_json_schema") || !strings.Contains(diagnostic, "uniqueItems is not permitted") {
		t.Fatalf("structured diagnostic = %q", diagnostic)
	}
}
