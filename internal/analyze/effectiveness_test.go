package analyze

import (
	"testing"

	"github.com/axcherednikov/codex-insights/internal/sessions"
)

func TestAggregateEffectivenessJoinsCompletedPriorTurnsAndAverages(t *testing.T) {
	var turns []sessions.Turn
	var taskTypes []TaskTypeResult
	var steering []SteeringResult
	for i := 0; i < 10; i++ {
		id := "b-" + string(rune('a'+i))
		turns = append(turns, sessions.Turn{ID: id, Status: "complete", Model: "model-b", Effort: "high", Tokens: 100, DurationMS: 2000, ToolCalls: 2})
		taskTypes = append(taskTypes, TaskTypeResult{TurnID: id, Type: "feature"})
		steering = append(steering, SteeringResult{PreviousTurnID: id, Label: "steering"})
	}
	for i := 0; i < 10; i++ {
		id := "a-" + string(rune('a'+i))
		turns = append(turns, sessions.Turn{ID: id, Status: "complete", Model: "model-a", Effort: "low", Tokens: 300, DurationMS: 4000, ToolCalls: 4})
		taskTypes = append(taskTypes, TaskTypeResult{TurnID: id, Type: "feature"})
		steering = append(steering, SteeringResult{PreviousTurnID: id, Label: "continuation"})
	}
	turns = append(turns, sessions.Turn{ID: "unlabeled", Status: "complete", Model: "model-z"})
	turns = append(turns, sessions.Turn{ID: "aborted", Status: "aborted", Model: "model-a"})

	result := AggregateEffectiveness(turns, taskTypes, steering)
	if result.Observed != 20 || result.Overall.Samples != 20 || result.Overall.Steering != 10 || len(result.ModelComparisons) != 2 {
		t.Fatalf("unexpected observed/comparisons: %d %#v", result.Observed, result.ModelComparisons)
	}
	if result.ModelComparisons[0].Model != "model-a" || result.ModelComparisons[1].Model != "model-b" {
		t.Fatalf("comparisons are not deterministic: %#v", result.ModelComparisons)
	}
	first := result.ModelComparisons[0].Stats
	if first.Samples != 10 || first.Steering != 0 || first.AverageTokens != 300 || first.AverageSeconds != 4 || first.AverageToolCalls != 4 {
		t.Fatalf("unexpected averages: %#v", first)
	}
}

func TestAggregateEffectivenessOmitsSubthresholdAndRequiresTwoModelCohorts(t *testing.T) {
	turns := []sessions.Turn{}
	types := []TaskTypeResult{}
	steering := []SteeringResult{}
	for i := 0; i < 19; i++ {
		id := "one-" + string(rune('a'+i))
		turns = append(turns, sessions.Turn{ID: id, Status: "complete", Model: "same", Effort: "low"})
		types = append(types, TaskTypeResult{TurnID: id, Type: "bugfix"})
		steering = append(steering, SteeringResult{PreviousTurnID: id, Label: "continuation"})
	}
	result := AggregateEffectiveness(turns, types, steering)
	if len(result.ModelComparisons) != 0 || len(result.SubagentGlobal) != 0 {
		t.Fatalf("subthreshold cohorts should be omitted: %#v", result)
	}
}

func TestAggregateEffectivenessDoesNotReportNegativeTokenAverages(t *testing.T) {
	result := AggregateEffectiveness(
		[]sessions.Turn{{ID: "one", Status: "complete", Tokens: -500}},
		[]TaskTypeResult{{TurnID: "one", Type: "research"}},
		[]SteeringResult{{PreviousTurnID: "one", Label: "continuation"}},
	)
	if result.Overall.AverageTokens != 0 {
		t.Fatalf("AverageTokens = %v", result.Overall.AverageTokens)
	}
}

func TestClassifierValidationRejectsUnsupportedEnums(t *testing.T) {
	if err := validateSteeringResults([]sessions.Followup{{PreviousTurnID: "one"}}, []SteeringResult{{PreviousTurnID: "one", Label: "bad"}}); err == nil {
		t.Fatal("expected unsupported steering label error")
	}
	if err := validateTaskTypeResults([]sessions.Interaction{{TurnID: "one"}}, []TaskTypeResult{{TurnID: "one", Type: "bad"}}); err == nil {
		t.Fatal("expected unsupported task type error")
	}
	if err := validateSteeringReasonResults([]sessions.Followup{{PreviousTurnID: "one"}}, []SteeringReasonResult{{PreviousTurnID: "one", Reason: "bad"}}); err == nil {
		t.Fatal("expected unsupported steering reason error")
	}
}
