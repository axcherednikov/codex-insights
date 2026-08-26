package judge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		return fmt.Errorf(
			"codex judge failed: %w\n%s",
			err,
			stderr.String(),
		)
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
