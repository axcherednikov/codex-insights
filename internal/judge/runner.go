package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const Marker = "[CODEX_INSIGHTS_JUDGE_V1]"

var ErrCLINotFound = errors.New("codex CLI not found in PATH")

type runnerError string

func (err runnerError) Error() string {
	return string(err)
}

const (
	errNilContext    runnerError = "judge context is nil"
	errInvalidModel  runnerError = "judge model is not a safe identifier"
	errInvalidEffort runnerError = "judge reasoning effort is unsupported"
	errInvalidPaths  runnerError = "judge files must remain inside their private directory"
)

const (
	judgeTempDirectoryPattern = "codex-insights-judge-*"
	judgeSchemaFile           = "schema.json"
	judgeResultFile           = "result.json"
	judgePrivateFileMode      = 0o600
	maxStderrDiagnosticBytes  = 512
	diagnosticFieldCapacity   = 2
)

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

func CheckCLI() error {
	if _, err := exec.LookPath("codex"); err != nil {
		return ErrCLINotFound
	}

	return nil
}

func (r Runner) Run(prompt string, schema any, result any) error {
	return r.RunContext(context.Background(), prompt, schema, result)
}

// RunContext runs the Judge subprocess for the lifetime of ctx.
func (r Runner) RunContext(ctx context.Context, prompt string, schema any, result any) error {
	if ctx == nil {
		return errNilContext
	}
	if err := validateRunner(r); err != nil {
		return err
	}
	if err := CheckCLI(); err != nil {
		return fmt.Errorf("check Judge CLI: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", judgeTempDirectoryPattern)
	if err != nil {
		return fmt.Errorf("create private Judge directory: %w", err)
	}
	tmpDir, err = filepath.Abs(tmpDir)
	if err != nil {
		cleanupErr := os.RemoveAll(tmpDir)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("remove private Judge directory: %w", cleanupErr)
		}

		return errors.Join(fmt.Errorf("resolve private Judge directory: %w", err), cleanupErr)
	}

	runErr := r.runInDirectory(ctx, tmpDir, prompt, schema, result)
	cleanupErr := os.RemoveAll(tmpDir)
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("remove private Judge directory: %w", cleanupErr)
	}

	return errors.Join(runErr, cleanupErr)
}

func (r Runner) runInDirectory(ctx context.Context, directory, prompt string, schema, result any) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open private Judge directory: %w", err)
	}

	operationErr := r.runWithRoot(ctx, root, directory, prompt, schema, result)
	closeErr := root.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close private Judge directory: %w", closeErr)
	}

	return errors.Join(operationErr, closeErr)
}

func (r Runner) runWithRoot(ctx context.Context, root *os.Root, directory, prompt string, schema, result any) error {
	schemaData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	if err := writeSchema(root, schemaData); err != nil {
		return err
	}

	fullPrompt := Marker + "\n\n" + prompt
	schemaPath := filepath.Join(directory, judgeSchemaFile)
	outputPath := filepath.Join(directory, judgeResultFile)
	if err := validateJudgePaths(directory, schemaPath, outputPath); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "codex")
	cmd.Args = []string{
		"codex",
		"exec",
		"--ephemeral",
		"-m", r.Model,
		"-c", "model_reasoning_effort=\"" + r.Effort + "\"",
		"-s", "read-only",
		"--skip-git-repo-check",
		"--output-schema", schemaPath,
		"-o", outputPath,
		"-",
	}

	cmd.Stdin = bytes.NewBufferString(fullPrompt)
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		commandErr := fmt.Errorf("codex judge failed: %w\n%s", err, sanitizeStderrForPrompt(stderr.String(), prompt))
		if contextErr := ctx.Err(); contextErr != nil {
			return errors.Join(commandErr, fmt.Errorf("judge command context ended: %w", contextErr))
		}

		return commandErr
	}

	data, err := root.ReadFile(judgeResultFile)
	if err != nil {
		return fmt.Errorf("read Judge output: %w", err)
	}

	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode judge output: %w", err)
	}

	return nil
}

func validateRunner(r Runner) error {
	if !safeModelIdentifier(r.Model) {
		return fmt.Errorf("validate Judge model: %w", errInvalidModel)
	}
	if !supportedEffort(r.Effort) {
		return fmt.Errorf("validate Judge effort: %w", errInvalidEffort)
	}

	return nil
}

func safeModelIdentifier(model string) bool {
	if model == "" || !asciiAlphaNumeric(model[0]) {
		return false
	}
	for index := 1; index < len(model); index++ {
		char := model[index]
		if !asciiAlphaNumeric(char) && char != '.' && char != '_' && char != '/' && char != '-' {
			return false
		}
	}

	return true
}

func asciiAlphaNumeric(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func supportedEffort(effort string) bool {
	switch effort {
	case "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func validateJudgePaths(directory, schemaPath, outputPath string) error {
	if !filepath.IsAbs(directory) || filepath.Dir(schemaPath) != directory || filepath.Dir(outputPath) != directory ||
		filepath.Base(schemaPath) != judgeSchemaFile || filepath.Base(outputPath) != judgeResultFile {
		return errInvalidPaths
	}

	return nil
}

func writeSchema(root *os.Root, data []byte) error {
	file, err := root.OpenFile(judgeSchemaFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, judgePrivateFileMode)
	if err != nil {
		return fmt.Errorf("create Judge schema: %w", err)
	}

	if err := file.Chmod(judgePrivateFileMode); err != nil {
		return errors.Join(fmt.Errorf("set Judge schema permissions: %w", err), wrapClose(file))
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		return errors.Join(fmt.Errorf("write Judge schema: %w", writeErr), wrapClose(file))
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Judge schema: %w", err)
	}

	return nil
}

func wrapClose(file *os.File) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Judge schema: %w", err)
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
			if len(line) > maxStderrDiagnosticBytes {
				line = line[:maxStderrDiagnosticBytes] + "..."
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
		parts := make([]string, 0, diagnosticFieldCapacity)
		if code != "" {
			parts = append(parts, code)
		}
		if message != "" {
			parts = append(parts, message)
		}
		diagnostic := strings.Join(parts, ": ")
		diagnostic = strings.Join(strings.Fields(diagnostic), " ")
		if len(diagnostic) > maxStderrDiagnosticBytes {
			diagnostic = diagnostic[:maxStderrDiagnosticBytes] + "..."
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
