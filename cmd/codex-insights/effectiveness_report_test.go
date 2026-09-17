package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func TestSummaryUsesJoinedEffectivenessPopulation(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	printHumanInsights(
		tr,
		analyze.EffectivenessAnalysis{Overall: analyze.EffectivenessStats{Samples: 1, Steering: 1}},
		[]analyze.SteeringResult{
			{PreviousTurnID: "joined", Label: "steering"},
			{PreviousTurnID: "unjoined", Label: "steering"},
		},
		nil, nil, nil, nil, nil, nil,
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "1 completed prior tasks") || strings.Contains(output, "steering: 200.0%") {
		t.Fatalf("summary did not use joined population: %s", output)
	}
}

func TestActionableInsightsExcludeOther(t *testing.T) {
	if got := topValidation([]analyze.ValidationResult{{ValidationType: "other"}}); got != "" {
		t.Fatalf("top validation selected other: %q", got)
	}
	if got := topPromptQuality([]analyze.PromptQualityResult{{Issue: "other"}}); got != "" {
		t.Fatalf("top prompt issue selected other: %q", got)
	}
	if got := topAgentsRule([]analyze.AgentsRuleResult{{Rule: "other"}}); got != "" {
		t.Fatalf("top AGENTS rule selected other: %q", got)
	}
	if got := topValidation([]analyze.ValidationResult{{ValidationType: "other"}, {ValidationType: "tests"}}); got != "tests" {
		t.Fatalf("top validation did not select actionable value: %q", got)
	}
}

func TestMeaningfulTaskTypeInsightsExcludeUnknownAndOther(t *testing.T) {
	stats := []analyze.TaskTypeEffectiveness{
		{TaskType: "unknown", Stats: analyze.EffectivenessStats{Samples: 20}},
		{TaskType: "other", Stats: analyze.EffectivenessStats{Samples: 20, Steering: 20}},
		{TaskType: "research", Stats: analyze.EffectivenessStats{Samples: 20, Steering: 5}},
	}
	if got := lowestMeaningfulTaskType(stats); got == nil || got.TaskType != "research" {
		t.Fatalf("lowest meaningful task type = %#v", got)
	}
	if got := highestMeaningfulTaskType(stats); got == nil || got.TaskType != "research" {
		t.Fatalf("highest meaningful task type = %#v", got)
	}
}

func TestMatchedRoutingHypothesisRequiresSameModelAndNoWorseLowerCost(t *testing.T) {
	comparisons := []analyze.ModelEffectiveness{
		{TaskType: "feature", Model: "model-a", Effort: "high", Stats: analyze.EffectivenessStats{Samples: 10, Steering: 6, AverageTokens: 300, AverageSeconds: 4, AverageToolCalls: 3}},
		{TaskType: "feature", Model: "model-a", Effort: "low", Stats: analyze.EffectivenessStats{Samples: 10, Steering: 5, AverageTokens: 200, AverageSeconds: 3, AverageToolCalls: 2}},
	}
	hypothesis := matchedRoutingHypothesis(comparisons)
	if hypothesis == nil || hypothesis.Lower.Effort != "low" || hypothesis.Other.Effort != "high" {
		t.Fatalf("unexpected routing hypothesis: %#v", hypothesis)
	}
	if got := matchedRoutingHypothesis([]analyze.ModelEffectiveness{
		comparisons[0],
		{TaskType: "feature", Model: "model-b", Effort: "low", Stats: comparisons[1].Stats},
	}); got != nil {
		t.Fatalf("different models should not produce hypothesis: %#v", got)
	}
	if got := matchedRoutingHypothesis([]analyze.ModelEffectiveness{
		comparisons[0],
		{TaskType: "feature", Model: "model-a", Effort: "low", Stats: analyze.EffectivenessStats{Samples: 10, Steering: 8, AverageTokens: 200, AverageSeconds: 3, AverageToolCalls: 2}},
	}); got != nil {
		t.Fatalf("worse steering should not produce hypothesis: %#v", got)
	}
	if got := matchedRoutingHypothesis([]analyze.ModelEffectiveness{
		comparisons[0],
		{TaskType: "feature", Model: "model-a", Effort: "low", Stats: analyze.EffectivenessStats{Samples: 10, Steering: 6, AverageTokens: 200, AverageSeconds: 5, AverageToolCalls: 2}},
	}); got != nil {
		t.Fatalf("mixed resource changes should not produce hypothesis: %#v", got)
	}
}

func TestRussianInsightTextDoesNotLeakClassifierKeys(t *testing.T) {
	tr, err := i18n.New("ru")
	if err != nil {
		t.Fatal(err)
	}
	got := insightText(tr, "reason", tr.SteeringReason("implementation_error"))
	if strings.Contains(got, "implementation_error") || strings.Contains(got, "steering") {
		t.Fatalf("Russian insight leaked internal key: %q", got)
	}
}
