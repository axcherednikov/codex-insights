package judge

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckCLIReportsMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := CheckCLI(); !errors.Is(err, ErrCLINotFound) {
		t.Fatalf("CheckCLI() error = %v, want ErrCLINotFound", err)
	}
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
