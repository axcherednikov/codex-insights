package analyze

import (
	"reflect"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/sessions"
)

func TestSubagentEvidencePreservesUserPopulationAndSeparatesResources(t *testing.T) {
	turns := []sessions.Turn{
		{ID: "one", Status: "complete", Tokens: 100, DurationMS: 3000, ToolCalls: 2},
		{ID: "two", Status: "complete", Tokens: 200, DurationMS: 6000, ToolCalls: 4},
		{ID: "one", Status: "complete", Tokens: 100}, // duplicate user turn
		{ID: "no-followup", Status: "complete"},
		{ID: "aborted", Status: "aborted"},
	}
	types := []TaskTypeResult{{TurnID: "one", Type: "bugfix"}, {TurnID: "two", Type: "research"}}
	labels := []SteeringResult{{PreviousTurnID: "one", Label: "steering"}, {PreviousTurnID: "two", Label: "continuation"}, {PreviousTurnID: "aborted", Label: "steering"}}
	resources := EffectivenessResources{
		RecordedTokens: ResourceMetric{Total: 110, KnownTurns: 1},
		Seconds:        ResourceMetric{Total: 5, KnownTurns: 1},
		Tools:          ResourceMetric{Total: 2, KnownTurns: 1},
	}
	evidence := SubagentEvidence{Tasks: map[string]SubagentTaskEvidence{
		"one": {AgentTurns: 2, Parent: EffectivenessResources{RecordedTokens: ResourceMetric{Total: 30, KnownTurns: 1}, Seconds: ResourceMetric{Total: 3, KnownTurns: 1}},
			Agents:   EffectivenessResources{RecordedTokens: ResourceMetric{Total: 220, KnownTurns: 2}, Seconds: ResourceMetric{Total: 10, KnownTurns: 2}},
			Profiles: []SubagentProfileEvidence{{Dimension: "role", Name: "reviewer", Resources: resources}, {Dimension: "role", Name: "reviewer", Resources: resources}, {Dimension: "model", Name: "actual", Resources: resources}}},
		"two":         {Parent: EffectivenessResources{EstimatedTokens: ResourceMetric{Total: 200, KnownTurns: 1}, Seconds: ResourceMetric{MissingTurns: 1}}},
		"no-followup": {AgentTurns: 1}, "aborted": {AgentTurns: 1}, "child-not-user": {AgentTurns: 1},
	}, LinkedTurns: 5, UnlinkedTurns: 2, ExcludedTurns: 1}
	old := AggregateEffectiveness(turns, types, labels)
	got := AggregateEffectiveness(turns, types, labels, evidence)
	if !reflect.DeepEqual(old.Overall, got.Overall) || !reflect.DeepEqual(old.TaskTypeStats, got.TaskTypeStats) || !reflect.DeepEqual(old.ModelComparisons, got.ModelComparisons) || got.Observed != 2 {
		t.Fatalf("legacy user population changed: %#v", got)
	}
	if old.SubagentDetails != nil || got.SubagentDetails.LinkedTurns != 5 || len(got.SubagentDetails.Profiles) != 2 {
		t.Fatalf("details or profile groups = %#v", got.SubagentDetails)
	}
	for _, profile := range got.SubagentDetails.Profiles {
		if profile.Stats.Samples != 1 || profile.Stats.Steering != 1 || profile.TaskType != "bugfix" {
			t.Fatalf("child turns counted as tasks: %#v", profile)
		}
		if profile.Dimension == "role" && (profile.Agents.RecordedTokens.Average() != 220 || profile.Agents.RecordedTokens.Tasks != 1) {
			t.Fatalf("repeated role turns not summed once per task: %#v", profile)
		}
	}
	for _, cohort := range got.SubagentDetails.Cohorts {
		if cohort.TaskType != "" {
			continue
		}
		if cohort.UsesSubagents {
			if cohort.Stats.Samples != 1 || cohort.Parent.RecordedTokens.Average() != 30 || cohort.Parent.Seconds.Average() != 3 || cohort.Agents.Seconds.Average() != 10 {
				t.Fatalf("parent and child totals mixed: %#v", cohort)
			}
		} else if cohort.Parent.RecordedTokens.Tasks != 0 || cohort.Parent.EstimatedTokens.Average() != 200 || cohort.Parent.Seconds.Tasks != 0 || cohort.Parent.Seconds.MissingTurns != 1 {
			t.Fatalf("missing metrics treated as zeros or token sources mixed: %#v", cohort)
		}
	}
}

func TestSubagentResourceAveragesUseKnownTaskDenominators(t *testing.T) {
	turns := []sessions.Turn{{ID: "one", Status: "complete", UsesSubagents: true}, {ID: "two", Status: "complete", UsesSubagents: true}}
	labels := []SteeringResult{{PreviousTurnID: "one", Label: "question"}, {PreviousTurnID: "two", Label: "user_correction"}}
	evidence := SubagentEvidence{Tasks: map[string]SubagentTaskEvidence{
		"one": {Agents: EffectivenessResources{RecordedTokens: ResourceMetric{Total: 100, KnownTurns: 1, MissingTurns: 1}, CounterMismatches: 1}},
		"two": {Agents: EffectivenessResources{RecordedTokens: ResourceMetric{MissingTurns: 1}, EstimatedTokens: ResourceMetric{Total: 300, KnownTurns: 1}}},
	}}
	got := AggregateEffectiveness(turns, nil, labels, evidence)
	for _, cohort := range got.SubagentDetails.Cohorts {
		if cohort.TaskType == "" && cohort.UsesSubagents {
			if cohort.Stats.Samples != 2 || cohort.Stats.Steering != 0 || cohort.Agents.RecordedTokens.Average() != 100 || cohort.Agents.RecordedTokens.Tasks != 1 || cohort.Agents.EstimatedTokens.Average() != 300 || cohort.Agents.RecordedTokens.MissingTurns != 2 || cohort.Agents.CounterMismatches != 1 {
				t.Fatalf("source averages or coverage: %#v", cohort)
			}
		}
	}
}

func TestSubagentRoutingStrataKeepTaskTypeModelAndEffortSeparate(t *testing.T) {
	turns := []sessions.Turn{
		{ID: "one", Status: "complete", Model: "same", Effort: "high", UsesSubagents: true},
		{ID: "two", Status: "complete", Model: "same", Effort: "high"},
		{ID: "three", Status: "complete", Model: "same", Effort: "low"},
		{ID: "unknown", Status: "complete"},
	}
	labels := []SteeringResult{}
	types := []TaskTypeResult{}
	for _, turn := range turns {
		labels = append(labels, SteeringResult{PreviousTurnID: turn.ID, Label: "continuation"})
		types = append(types, TaskTypeResult{TurnID: turn.ID, Type: "bugfix"})
	}
	got := AggregateEffectiveness(turns, types, labels, SubagentEvidence{})
	if len(got.SubagentDetails.RoutingCohorts) != 4 {
		t.Fatalf("strata: %#v", got.SubagentDetails.RoutingCohorts)
	}
	for _, cohort := range got.SubagentDetails.RoutingCohorts {
		if cohort.Name == "same / high" && cohort.Stats.Samples != 1 {
			t.Fatalf("routing samples combined: %#v", cohort)
		}
		if cohort.Name == "same / low" && cohort.UsesSubagents && cohort.Stats.Samples != 0 {
			t.Fatal("low effort counted as high")
		}
	}
}

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
