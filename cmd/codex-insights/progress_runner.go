package main

import (
	"fmt"

	"github.com/axcherednikov/codex-insights/internal/analyze"
)

type progressGuardRunner struct {
	runner   analyze.SemanticRunner
	renderer *semanticProgressRenderer
}

func (runner progressGuardRunner) Run(prompt string, schema any, result any) error {
	if rendererErr := runner.renderer.Err(); rendererErr != nil {
		return fmt.Errorf("stop Judge after failed privacy disclosure: %w", rendererErr)
	}
	if runner.runner == nil {
		return errMissingJudgeRunner
	}

	if err := runner.runner.Run(prompt, schema, result); err != nil {
		return fmt.Errorf("run semantic Judge request: %w", err)
	}

	return nil
}
