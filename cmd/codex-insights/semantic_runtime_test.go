package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

type disclosureCheckingRunner struct {
	output *bytes.Buffer
	seen   bool
}

func (r *disclosureCheckingRunner) Run(string, any, any) error {
	r.seen = strings.Contains(r.output.String(), "Selected task prompts")
	return errors.New("permission denied")
}

func TestValidateSemanticConcurrency(t *testing.T) {
	for _, value := range []int{0, -1, maxSemanticConcurrency + 1} {
		if err := validateSemanticConcurrency(value); err == nil {
			t.Fatalf("concurrency %d unexpectedly accepted", value)
		}
	}
	for _, value := range []int{1, analyze.SemanticDefaultWorkers, maxSemanticConcurrency} {
		if err := validateSemanticConcurrency(value); err != nil {
			t.Fatalf("concurrency %d rejected: %v", value, err)
		}
	}
}

func TestConvertSemanticAnalysisCoversFollowupsAndOmitsUnavailableTaskType(t *testing.T) {
	followups := []sessions.Followup{{PreviousTurnID: "empty", TurnID: "next"}}
	result := analyze.SemanticResult{
		TurnID:                   "empty",
		FollowupLabel:            "steering",
		FollowupConfidence:       .9,
		SteeringReason:           "implementation_error",
		SteeringReasonConfidence: .9,
		PreventionMechanisms:     []string{"validation", "task_prompt", "agents_md", "skill"},
		PreventionConfidence:     .9,
		PromptIssue:              "missing_context", PromptIssueConfidence: .9,
		AgentsRule: "avoid_scope_creep", AgentsRuleConfidence: .9,
		SkillCandidate: "verification_workflow", SkillConfidence: .9,
		ValidationType: "tests", ValidationConfidence: .9,
	}
	converted, err := convertSemanticAnalysis(analyze.SemanticAnalysis{Results: []analyze.SemanticResult{
		{TurnID: "available", TaskType: "research", TaskConfidence: .8},
		result,
	}}, followups)
	if err != nil {
		t.Fatal(err)
	}
	if len(converted.taskTypes) != 1 || converted.taskTypes[0].TurnID != "available" {
		t.Fatalf("task types = %#v", converted.taskTypes)
	}
	if len(converted.steering) != len(followups) || len(converted.reasons) != 1 || len(converted.prevention) != 1 || len(converted.promptQuality) != 1 || len(converted.agentsRules) != 1 || len(converted.skillCandidates) != 1 || len(converted.validation) != 1 {
		t.Fatalf("converted follow-up outputs = %#v", converted)
	}
	if _, err := convertSemanticAnalysis(analyze.SemanticAnalysis{Results: []analyze.SemanticResult{{TurnID: "empty"}}}, followups); err == nil {
		t.Fatal("missing explicit follow-up result was accepted")
	}
}

func TestSemanticProgressSuppressesWarmRunsAndBoundsNonTTYOutput(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	var warm bytes.Buffer
	warmRenderer := newSemanticProgressRenderer(tr, &warm, false)
	warmRenderer.Update(analyze.SemanticProgress{TotalRecords: 4, CompletedRecords: 4, CacheHits: 4})
	warmRenderer.Finish()
	if warm.Len() != 0 {
		t.Fatalf("warm progress was not suppressed: %q", warm.String())
	}

	var cold bytes.Buffer
	coldRenderer := newSemanticProgressRenderer(tr, &cold, false)
	coldRenderer.Update(analyze.SemanticProgress{TotalRecords: 100, CompletedRecords: 0, ConfiguredWorkers: 6})
	for _, completed := range []int{1, 10, 25, 50, 75, 100} {
		coldRenderer.Update(analyze.SemanticProgress{TotalRecords: 100, CompletedRecords: completed, CacheHits: 2, ConfiguredWorkers: 6, Elapsed: time.Second})
	}
	coldRenderer.Finish()
	if !strings.Contains(cold.String(), "Selected task prompts") || !strings.Contains(cold.String(), "100% (100/100)") {
		t.Fatalf("cold disclosure/progress missing: %q", cold.String())
	}
	if lines := strings.Count(cold.String(), "Semantic analysis:"); lines > 12 {
		t.Fatalf("non-TTY progress was unbounded: %d lines", lines)
	}

	ru, err := i18n.New("ru")
	if err != nil {
		t.Fatal(err)
	}
	var russian bytes.Buffer
	ruRenderer := newSemanticProgressRenderer(ru, &russian, false)
	ruRenderer.Update(analyze.SemanticProgress{TotalRecords: 2, ConfiguredWorkers: 2})
	ruRenderer.Update(analyze.SemanticProgress{TotalRecords: 2, CompletedRecords: 2, CacheHits: 1, ConfiguredWorkers: 2, Elapsed: time.Second})
	ruText := russian.String()
	if !strings.Contains(ruText, "Выбранные постановки задач") || !strings.Contains(ruText, "Семантический анализ") {
		t.Fatalf("Russian cold progress was not localized: %q", ruText)
	}
	for _, internal := range []string{"steering", "implementation_error", "cache hits"} {
		if strings.Contains(ruText, internal) {
			t.Fatalf("Russian cold progress leaked %q: %q", internal, ruText)
		}
	}
}

func TestFormatDurationIsCompactAndDeterministic(t *testing.T) {
	if got := formatDuration(500 * time.Microsecond); got != "0ms" {
		t.Fatalf("sub-millisecond duration = %q", got)
	}
	if got := formatDuration(1250 * time.Millisecond); got != "1.25s" {
		t.Fatalf("duration = %q", got)
	}
}

func TestSemanticTimingTextUsesLocalizedLabels(t *testing.T) {
	tr, err := i18n.New("ru")
	if err != nil {
		t.Fatal(err)
	}
	text := semanticTimingText(tr, analysisTimings{cacheLookup: time.Second}, analyze.SemanticStats{
		Methodology: "semantic-v1", PromptVersion: "prompt-v1", SchemaVersion: "schema-v1", Model: "model", Effort: "medium",
	})
	if !strings.Contains(text, "поиск в кеше: 1s") || !strings.Contains(text, "Методология: semantic-v1") {
		t.Fatalf("timing labels were not localized: %q", text)
	}
	if strings.Contains(text, "timing_cache_lookup") {
		t.Fatalf("internal timing key leaked: %q", text)
	}
}

func TestFinalReportDurationExcludesSemanticDuration(t *testing.T) {
	if got := finalReportDuration(3*time.Millisecond, 7*time.Millisecond); got != 10*time.Millisecond {
		t.Fatalf("final report duration = %s", got)
	}
	semanticDuration := 5 * time.Minute
	if got := finalReportDuration(3*time.Millisecond, 7*time.Millisecond); got >= semanticDuration {
		t.Fatalf("report timing included semantic duration: %s", got)
	}
}

func TestDisclosureIsWrittenBeforeFirstJudgeCall(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	runner := &disclosureCheckingRunner{output: &output}
	renderer := newSemanticProgressRenderer(tr, &output, false)
	config := analyze.SemanticConfig{Runner: runner, Workers: 1, BatchSize: 1, MaxRetries: 0, Progress: renderer.Update}
	_, err = analyze.NewSemanticEngine(config).Analyze([]analyze.SemanticInput{{TurnID: "one", Prompt: "private"}})
	renderer.Finish()
	if err == nil {
		t.Fatal("expected injected Judge failure")
	}
	if !runner.seen {
		t.Fatalf("privacy disclosure was not written before Judge call: %q", output.String())
	}
}

func TestNewSemanticCacheHonorsExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "isolated", "semantic-cache.json")
	store, err := newSemanticCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("test", map[string]string{"value": "label"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("explicit cache path was not used: %v", err)
	}
}
