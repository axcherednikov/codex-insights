package judge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const Marker = "[CODEX_INSIGHTS_JUDGE_V1]"

type Runner struct {
	Model  string
	Effort string
}

func New() Runner {
	return Runner{
		Model:  "gpt-5.6-sol",
		Effort: "medium",
	}
}

func (r Runner) Run(prompt string, schema any, result any) error {
	tmpDir, err := os.MkdirTemp("", "codex-insights-judge-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	schemaPath := filepath.Join(tmpDir, "schema.json")
	outputPath := filepath.Join(tmpDir, "result.json")

	schemaData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	if err := os.WriteFile(schemaPath, schemaData, 0o600); err != nil {
		return fmt.Errorf("write schema: %w", err)
	}

	fullPrompt := Marker + "\n\n" + prompt

	cmd := exec.Command(
		"codex",
		"exec",
		"-m", r.Model,
		"-c", fmt.Sprintf(`model_reasoning_effort="%s"`, r.Effort),
		"-s", "read-only",
		"--skip-git-repo-check",
		"--output-schema", schemaPath,
		"-o", outputPath,
		"-",
	)

	cmd.Stdin = bytes.NewBufferString(fullPrompt)
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex judge failed: %w\n%s", err, sanitizeStderrForPrompt(stderr.String(), prompt))
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return fmt.Errorf("read judge output: %w", err)
	}

	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode judge output: %w", err)
	}

	return nil
}

const privateStderrMessage = "stderr omitted for privacy"

// sanitizeStderr keeps only a compact actionable ERROR diagnostic. Codex
// providers may echo stdin in stderr, so arbitrary stderr must never be
// included in an error returned to callers.
func sanitizeStderr(stderr string) string {
	lines := strings.Split(stderr, "\n")
	if diagnostic, ok := structuredErrorDiagnostic(lines); ok {
		return diagnostic
	}
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "ERROR") || strings.HasPrefix(upper, "[ERROR]") {
			if marker := strings.Index(strings.ToUpper(line), "RECORDS:"); marker >= 0 {
				line = strings.TrimSpace(line[:marker])
			}
			if len(line) > 512 {
				line = line[:512] + "..."
			}
			return line
		}
	}
	return privateStderrMessage
}

func structuredErrorDiagnostic(lines []string) (string, bool) {
	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		upper := strings.ToUpper(line)
		if !strings.HasPrefix(upper, "ERROR") && !strings.HasPrefix(upper, "[ERROR]") {
			continue
		}
		brace := strings.Index(line, "{")
		if brace < 0 {
			continue
		}
		var object map[string]any
		decoder := json.NewDecoder(strings.NewReader(strings.Join(append([]string{line[brace:]}, lines[i+1:]...), "\n")))
		if err := decoder.Decode(&object); err != nil {
			continue
		}
		diagnosticObject := object
		if nested, ok := object["error"].(map[string]any); ok {
			diagnosticObject = nested
		}
		code := firstDiagnosticString(diagnosticObject, "code", "type", "error_code")
		message := firstDiagnosticString(diagnosticObject, "message", "detail", "error")
		if code == "" && message == "" {
			continue
		}
		parts := make([]string, 0, 2)
		if code != "" {
			parts = append(parts, code)
		}
		if message != "" {
			parts = append(parts, message)
		}
		diagnostic := strings.Join(parts, ": ")
		diagnostic = strings.Join(strings.Fields(diagnostic), " ")
		if len(diagnostic) > 512 {
			diagnostic = diagnostic[:512] + "..."
		}
		return diagnostic, true
	}
	return "", false
}

func firstDiagnosticString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeStderrForPrompt(stderr, prompt string) string {
	diagnostic := sanitizeStderr(stderr)
	if prompt != "" {
		diagnostic = strings.ReplaceAll(diagnostic, prompt, "[prompt omitted]")
	}
	return diagnostic
}
