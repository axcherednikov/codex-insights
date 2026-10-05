package golden

import (
	"testing"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

func TestSemanticSnapshotFromResultsUsesLabeledFollowupsOnly(t *testing.T) {
	followups := []sessions.Followup{
		{PreviousTurnID: "turn-a"},
		{PreviousTurnID: "turn-b"},
		{PreviousTurnID: "turn-c"},
		{PreviousTurnID: "missing"},
	}
	results := []analyze.SemanticResult{
		{TurnID: "turn-a", FollowupLabel: "steering", TaskType: "refactor"},
		{TurnID: "turn-b", FollowupLabel: "continuation", TaskType: "refactor"},
		{TurnID: "turn-c", TaskType: "feature"},
	}
	got := SemanticSnapshotFromResults(followups, results)
	if got.Samples != 2 || got.SteeringRate != 50 {
		t.Fatalf("snapshot samples/rate = %d/%.1f, want 2/50.0", got.Samples, got.SteeringRate)
	}
	if got.Cohorts["refactor"] != (CohortRate{Samples: 2, SteeringRate: 50}) {
		t.Fatalf("refactor cohort = %#v", got.Cohorts["refactor"])
	}
	if _, ok := got.Cohorts["feature"]; ok {
		t.Fatalf("unlabeled follow-up created feature cohort: %#v", got.Cohorts)
	}
}
