package golden

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/axcherednikov/codex-insights/internal/analyze"
)

func TestRedactAndCandidateAnonymization(t *testing.T) {
	inputs := []ExportInput{{ID: "local-turn-secret", Prompt: "email a@example.com token=abc123 /Users/alice/project\n```go\nsecret()\n```", FollowupPrompt: strings.Repeat("x", MaxTextLength+100)}}
	fixture := BuildCandidate(inputs, 1, Methodology{MethodologyVersion: "m", PromptVersion: "p", SchemaVersion: "s"})
	if fixture.Cases[0].ID == inputs[0].ID || strings.Contains(fixture.Cases[0].Prompt, "a@example.com") || strings.Contains(fixture.Cases[0].Prompt, "/Users/alice") || strings.Contains(fixture.Cases[0].Prompt, "secret()") {
		t.Fatalf("candidate was not redacted: %#v", fixture.Cases[0])
	}
	if len(fixture.Cases[0].FollowupPrompt) > MaxTextLength+32 {
		t.Fatalf("text cap not applied: %d", len(fixture.Cases[0].FollowupPrompt))
	}
	if fixture.Provenance.Status != ApprovalCandidate || fixture.Cases[0].Expected != nil {
		t.Fatal("candidate has invalid approval/expected state")
	}
	if got := Redact("начало " + strings.Repeat("ю", MaxTextLength) + " конец"); !utf8.ValidString(got) {
		t.Fatal("rune cap produced invalid UTF-8")
	}
}

func TestWriteUsesRestrictivePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "fixture.json")
	if err := Write(path, Fixture{FormatVersion: FormatVersion, Methodology: Methodology{MethodologyVersion: "m", PromptVersion: "p", SchemaVersion: "s"}, Provenance: Provenance{Status: ApprovalCandidate}, Cases: []Case{{ID: "case-000001", Prompt: "x"}}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses inherited ACLs rather than POSIX permission bits")
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
}

func TestValidateRejectsUnapprovedAndMalformed(t *testing.T) {
	fixture := Fixture{FormatVersion: FormatVersion, Methodology: Methodology{MethodologyVersion: "m", PromptVersion: "p", SchemaVersion: "s"}, Provenance: Provenance{Status: ApprovalCandidate}, Cases: []Case{{ID: "case-000001", Prompt: "x"}}}
	if err := fixture.ValidateForEvaluation(); err == nil {
		t.Fatal("candidate fixture accepted")
	}
	fixture.Provenance = Provenance{Status: ApprovalApproved, ApprovedBy: "reviewer", ApprovedAt: "2026-01-01T00:00:00Z", Source: "review"}
	fixture.Cases[0].Expected = &Expected{TaskType: "not-an-enum"}
	if err := fixture.ValidateForEvaluation(); err == nil {
		t.Fatal("malformed expected enum accepted")
	}
}

func TestReadRejectsUnknownAndTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	valid := `{"format_version":"golden-v1","methodology":{"methodology_version":"m","prompt_version":"p","schema_version":"s"},"provenance":{"status":"candidate"},"cases":[{"id":"case-000001","prompt":"x"}]}`
	if err := os.WriteFile(path, []byte(strings.Replace(valid, `"cases"`, `"unknown":true,"cases"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("unknown fixture field accepted")
	}
	if err := os.WriteFile(path, []byte(valid+" {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestValidationRequiresRealExpectedAndInputs(t *testing.T) {
	base := Fixture{FormatVersion: FormatVersion, Methodology: Methodology{MethodologyVersion: "m", PromptVersion: "p", SchemaVersion: "s"}, Provenance: Provenance{Status: ApprovalApproved, ApprovedBy: "reviewer", ApprovedAt: "2026-01-01T00:00:00Z", Source: "review"}}
	base.Cases = []Case{{ID: "case-000001", Prompt: "x", Expected: &Expected{}}}
	if err := base.ValidateForEvaluation(); err == nil {
		t.Fatal("empty expected object accepted")
	}
	base.Cases = []Case{{ID: "case-000001", Prompt: "x", Expected: &Expected{TaskType: "feature"}}, {ID: "case-000002", Prompt: "y"}}
	if err := base.ValidateForEvaluation(); err == nil {
		t.Fatal("unlabeled approved case accepted")
	}
	base.Cases = []Case{{ID: "case-000001", PreviousAnswer: "answer", Expected: &Expected{TaskType: "feature"}}}
	if err := base.ValidateForEvaluation(); err == nil {
		t.Fatal("previous answer treated as task input")
	}
	base.Cases = []Case{{ID: "case-000001", FollowupPrompt: "next", Expected: &Expected{TaskType: "feature"}}}
	if err := base.ValidateForEvaluation(); err == nil {
		t.Fatal("task type accepted without prompt")
	}
	base.Cases = []Case{{ID: "case-000001", Prompt: "x", Expected: &Expected{FollowupLabel: "continuation"}}}
	if err := base.ValidateForEvaluation(); err == nil {
		t.Fatal("followup expected field accepted without followup")
	}
	base.Cases = []Case{{ID: "case-000001", Prompt: "x", FollowupPrompt: "next", Expected: &Expected{PromptIssue: "missing_context"}}}
	if err := base.ValidateForEvaluation(); err != nil {
		t.Fatalf("partial conditional label without mechanism rejected: %v", err)
	}
}

func TestApprovedFixtureMetadataDriftRemainsEvaluable(t *testing.T) {
	fixture := Fixture{FormatVersion: FormatVersion, Methodology: Methodology{MethodologyVersion: "old-method", PromptVersion: "old-prompt", SchemaVersion: "old-schema"}, Provenance: Provenance{Status: ApprovalApproved, ApprovedBy: "reviewer", ApprovedAt: "2026-01-01T00:00:00Z", Source: "review"}, Cases: []Case{{ID: "case-000001", Prompt: "x", Expected: &Expected{TaskType: "feature"}}}}
	if err := fixture.ValidateForEvaluation(); err != nil {
		t.Fatalf("metadata drift should warn at CLI, not refuse evaluation: %v", err)
	}
}

func TestApprovedEmptyPreventionSetRoundTripsAsEmptyArray(t *testing.T) {
	empty := []string{}
	fixture := Fixture{FormatVersion: FormatVersion, Methodology: Methodology{MethodologyVersion: "m", PromptVersion: "p", SchemaVersion: "s"}, Provenance: Provenance{Status: ApprovalApproved, ApprovedBy: "reviewer", ApprovedAt: "2026-01-01T00:00:00Z", Source: "review"}, Cases: []Case{{ID: "case-000001", Prompt: "x", FollowupPrompt: "next", Expected: &Expected{PreventionMechanisms: &empty}}}}
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := Write(path, fixture); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"prevention_mechanisms": []`) {
		t.Fatalf("empty prevention set was omitted: %s", data)
	}
	read, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if read.Cases[0].Expected == nil || read.Cases[0].Expected.PreventionMechanisms == nil || len(*read.Cases[0].Expected.PreventionMechanisms) != 0 {
		t.Fatalf("empty prevention set did not round-trip: %#v", read.Cases[0].Expected)
	}
}

func TestEvaluateMetricsPartialAndStableConfusions(t *testing.T) {
	prevention := []string{"validation", "skill"}
	f := Fixture{Cases: []Case{{ID: "case-000001", Prompt: "a", Expected: &Expected{TaskType: "feature", PreventionMechanisms: &prevention}}, {ID: "case-000002", Prompt: "b", Expected: &Expected{TaskType: "bugfix"}}, {ID: "case-000003", Prompt: "c"}}}
	results := []analyze.SemanticResult{{TurnID: "case-000001", TaskType: "bugfix", PreventionMechanisms: []string{"validation"}}, {TurnID: "case-000002", TaskType: "bugfix"}, {TurnID: "case-000003", TaskType: "feature"}}
	metrics, err := EvaluateMetrics(f, results)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Fields["task_type"].Samples != 2 || metrics.Fields["task_type"].Correct != 1 {
		t.Fatalf("task metrics = %#v", metrics.Fields["task_type"])
	}
	if metrics.Prevention.Samples != 1 || metrics.Prevention.ExactMatches != 0 || metrics.Prevention.Recall != .5 {
		t.Fatalf("prevention metrics = %#v", metrics.Prevention)
	}
	if len(metrics.Confusions) != 2 {
		t.Fatalf("confusions = %#v", metrics.Confusions)
	}
	if metrics.Confusions[0].Field != "prevention" || metrics.Confusions[1].Field != "task_type" {
		t.Fatalf("confusions not stably sorted = %#v", metrics.Confusions)
	}
	if metrics.Confusions[1].Expected != "feature" || metrics.Confusions[1].Predicted != "bugfix" {
		t.Fatalf("task confusion = %#v", metrics.Confusions[1])
	}
}

func TestEvaluateMetricsJoinsResultsByCaseID(t *testing.T) {
	f := Fixture{Cases: []Case{{ID: "case-000001", Prompt: "a", Expected: &Expected{TaskType: "feature"}}, {ID: "case-000002", Prompt: "b", Expected: &Expected{TaskType: "bugfix"}}}}
	results := []analyze.SemanticResult{{TurnID: "case-000002", TaskType: "bugfix"}, {TurnID: "case-000001", TaskType: "feature"}}
	metrics, err := EvaluateMetrics(f, results)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Fields["task_type"].Correct != 2 {
		t.Fatalf("reordered results scored incorrectly: %#v", metrics.Fields["task_type"])
	}
	results[1].TurnID = "case-000002"
	if _, err := EvaluateMetrics(f, results); err == nil {
		t.Fatal("duplicate result IDs accepted")
	}
	results[1].TurnID = "case-missing"
	if _, err := EvaluateMetrics(f, results); err == nil {
		t.Fatal("unexpected/missing result IDs accepted")
	}
}

func TestHistoricalGuardExactAndToleranceBoundaries(t *testing.T) {
	before, _ := time.Parse(time.RFC3339, HistoricalBefore)
	opts := HistoricalOptions{Days: 0, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}
	actual := DeterministicSnapshot{Tasks: 1046, Complete: 1023, Aborted: 20, Incomplete: 3, Followups: 930, AverageTokens: 1652761, AverageSeconds: 258.3, AverageTools: 15.6}
	semantic := SemanticSnapshot{Samples: 930, SteeringRate: 14.8, Cohorts: map[string]CohortRate{"refactor": {Samples: 10, SteeringRate: 30}, "architecture": {Samples: 10, SteeringRate: 20.6}}}
	if result := CheckHistoricalGuard(opts, actual, semantic); !result.Pass() {
		t.Fatalf("exact guard = %#v", result)
	}
	semantic.SteeringRate += HistoricalSteeringTolerancePP
	semantic.Cohorts["refactor"] = CohortRate{Samples: 10, SteeringRate: 30 + HistoricalCohortTolerancePP}
	result := CheckHistoricalGuard(opts, actual, semantic)
	if len(result.SemanticWarnings) != 0 {
		t.Fatalf("boundary should pass: %#v", result)
	}
	semantic.SteeringRate += .01
	semantic.Cohorts["architecture"] = CohortRate{Samples: 10, SteeringRate: 20.6 - HistoricalCohortTolerancePP - .01}
	result = CheckHistoricalGuard(opts, actual, semantic)
	if len(result.SemanticWarnings) != 2 {
		t.Fatalf("expected overall/cohort warnings: %#v", result)
	}
}

func TestHistoricalGuardDeterministicMismatchAndSelection(t *testing.T) {
	before, _ := time.Parse(time.RFC3339, HistoricalBefore)
	opts := HistoricalOptions{Days: 0, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}
	actual := DeterministicSnapshot{Tasks: 1, Complete: 1}
	result := CheckHistoricalGuard(opts, actual, SemanticSnapshot{})
	if !result.Fail() || len(result.DeterministicMismatches) != 8 {
		t.Fatalf("mismatch result = %#v", result)
	}
	if CheckHistoricalGuard(HistoricalOptions{Days: 1, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}, actual, SemanticSnapshot{}).Applicable {
		t.Fatal("non-snapshot invocation selected")
	}
}

func TestHistoricalGuardReportsEachDeterministicField(t *testing.T) {
	before, _ := time.Parse(time.RFC3339, HistoricalBefore)
	opts := HistoricalOptions{Days: 0, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}
	base := DeterministicSnapshot{Tasks: 1046, Complete: 1023, Aborted: 20, Incomplete: 3, Followups: 930, AverageTokens: 1652761, AverageSeconds: 258.3, AverageTools: 15.6}
	cases := []struct {
		name   string
		mutate func(*DeterministicSnapshot)
	}{
		{"tasks", func(v *DeterministicSnapshot) { v.Tasks++ }}, {"complete", func(v *DeterministicSnapshot) { v.Complete++ }},
		{"aborted", func(v *DeterministicSnapshot) { v.Aborted++ }}, {"incomplete", func(v *DeterministicSnapshot) { v.Incomplete++ }},
		{"followups", func(v *DeterministicSnapshot) { v.Followups++ }}, {"average tokens", func(v *DeterministicSnapshot) { v.AverageTokens++ }},
		{"average seconds", func(v *DeterministicSnapshot) { v.AverageSeconds += .1 }}, {"average tools", func(v *DeterministicSnapshot) { v.AverageTools += .1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := base
			tc.mutate(&actual)
			result := CheckHistoricalGuard(opts, actual, SemanticSnapshot{SteeringRate: 14.8})
			if len(result.DeterministicMismatches) != 1 {
				t.Fatalf("mismatches = %#v", result.DeterministicMismatches)
			}
		})
	}
}

func TestHistoricalGuardWarningOrdering(t *testing.T) {
	before, _ := time.Parse(time.RFC3339, HistoricalBefore)
	opts := HistoricalOptions{Days: 0, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}
	semantic := SemanticSnapshot{Samples: 930, SteeringRate: 0, Cohorts: map[string]CohortRate{"architecture": {Samples: 20, SteeringRate: 0}, "refactor": {Samples: 20, SteeringRate: 100}}}
	result := CheckHistoricalGuard(opts, DeterministicSnapshot{Tasks: 1046, Complete: 1023, Aborted: 20, Incomplete: 3, Followups: 930, AverageTokens: 1652761, AverageSeconds: 258.3, AverageTools: 15.6}, semantic)
	if len(result.SemanticWarnings) != 3 {
		t.Fatalf("warnings = %#v", result.SemanticWarnings)
	}
	for i := 1; i < len(result.SemanticWarnings); i++ {
		if result.SemanticWarnings[i-1] > result.SemanticWarnings[i] {
			t.Fatalf("warnings not sorted: %#v", result.SemanticWarnings)
		}
	}
}

func TestHistoricalGuardWarnsOnMissingOrSmallReferenceCohortAndSampleCount(t *testing.T) {
	before, _ := time.Parse(time.RFC3339, HistoricalBefore)
	opts := HistoricalOptions{Days: 0, Before: before, LegacyExcludeOriginator: HistoricalLegacyOriginator}
	result := CheckHistoricalGuard(opts, DeterministicSnapshot{}, SemanticSnapshot{Samples: 929, SteeringRate: 14.8, Cohorts: map[string]CohortRate{"refactor": {Samples: 9, SteeringRate: 30}}})
	if len(result.SemanticWarnings) != 3 {
		t.Fatalf("expected sample, small refactor, and missing architecture warnings: %#v", result.SemanticWarnings)
	}
	if !strings.Contains(strings.Join(result.SemanticWarnings, "\n"), "sample count") || !strings.Contains(strings.Join(result.SemanticWarnings, "\n"), "architecture cohort missing") || !strings.Contains(strings.Join(result.SemanticWarnings, "\n"), "refactor cohort has 9") {
		t.Fatalf("missing cohort/sample warnings: %#v", result.SemanticWarnings)
	}
}
