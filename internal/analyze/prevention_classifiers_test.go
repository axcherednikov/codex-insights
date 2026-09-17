package analyze

import (
	"strings"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/sessions"
)

func testFollowups() []sessions.Followup {
	return []sessions.Followup{
		{PreviousTurnID: "one", PreviousAnswer: "answer one", Prompt: "prompt one"},
		{PreviousTurnID: "two", PreviousAnswer: "answer two", Prompt: "prompt two"},
		{PreviousTurnID: "three", PreviousAnswer: "answer three", Prompt: "prompt three"},
	}
}

func TestPreventionCasesFilterAndPreserveOrder(t *testing.T) {
	cases := preventionCases(testFollowups(), []PreventionResult{
		{PreviousTurnID: "three", Applicable: []string{"skill"}},
		{PreviousTurnID: "one", Applicable: []string{"skill", "task_prompt"}},
		{PreviousTurnID: "one", Applicable: []string{"skill"}},
	}, "skill")
	if len(cases) != 2 || cases[0].PreviousTurnID != "one" || cases[1].PreviousTurnID != "three" {
		t.Fatalf("unexpected filtered cases: %#v", cases)
	}
}

func TestPromptQualityValidationRejectsIncompleteDuplicateAndUnsupported(t *testing.T) {
	cases := testFollowups()[:2]
	tests := []struct {
		name    string
		results []PromptQualityResult
		want    string
	}{
		{name: "incomplete", results: []PromptQualityResult{{PreviousTurnID: "one", Issue: "other"}}, want: "expected 2"},
		{name: "duplicate", results: []PromptQualityResult{{PreviousTurnID: "one", Issue: "other"}, {PreviousTurnID: "one", Issue: "other"}}, want: "duplicate"},
		{name: "unsupported", results: []PromptQualityResult{{PreviousTurnID: "one", Issue: "unsupported"}, {PreviousTurnID: "two", Issue: "other"}}, want: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePromptQualityResults(cases, test.results)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestAgentsAndSkillValidationRejectUnsupported(t *testing.T) {
	cases := testFollowups()[:1]
	if err := validateAgentsRulesResults(cases, []AgentsRuleResult{{PreviousTurnID: "one", Rule: "not-a-rule"}}); err == nil {
		t.Fatal("expected unsupported AGENTS.md rule error")
	}
	if err := validateSkillCandidatesResults(cases, []SkillCandidateResult{{PreviousTurnID: "one", Category: "not-a-category"}}); err == nil {
		t.Fatal("expected unsupported Skill category error")
	}
}

func TestClassifierSchemasAreStrictAndEnumerated(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"prompt": promptQualitySchema(),
		"agents": agentsRulesSchema(),
		"skills": skillCandidatesSchema(),
	} {
		if schema["additionalProperties"] != false {
			t.Errorf("%s schema is not strict", name)
		}
		properties := schema["properties"].(map[string]any)
		items := properties["results"].(map[string]any)["items"].(map[string]any)
		if items["additionalProperties"] != false {
			t.Errorf("%s item schema is not strict", name)
		}
	}
}

func TestVersionedClassifierCacheKeysAreStableAndInputSpecific(t *testing.T) {
	item := testFollowups()[0]
	if promptQualityCacheKey(item) != promptQualityCacheKey(item) {
		t.Fatal("prompt quality cache key is not stable")
	}
	if promptQualityCacheKey(item) == promptQualityCacheKey(testFollowups()[1]) {
		t.Fatal("prompt quality cache key does not include input")
	}
	if agentsRulesCacheKey(item) == skillCandidatesCacheKey(item) {
		t.Fatal("classifier cache namespaces must be distinct")
	}
}

func TestClassifiersSkipJudgeWhenNoApplicableCases(t *testing.T) {
	followups := testFollowups()
	prevention := []PreventionResult{{PreviousTurnID: "one", Applicable: []string{"validation"}}}

	promptQuality, err := (PromptQualityAnalyzer{batchSize: 10}).Analyze(followups, prevention)
	if err != nil || len(promptQuality.Results) != 0 || promptQuality.Evaluated != 0 {
		t.Fatalf("prompt quality no-case analysis = %#v, err = %v", promptQuality, err)
	}
	agentsRules, err := (AgentsRulesAnalyzer{batchSize: 10}).Analyze(followups, prevention)
	if err != nil || len(agentsRules.Results) != 0 || agentsRules.Evaluated != 0 {
		t.Fatalf("AGENTS.md no-case analysis = %#v, err = %v", agentsRules, err)
	}
	skillCandidates, err := (SkillCandidatesAnalyzer{batchSize: 10}).Analyze(followups, prevention)
	if err != nil || len(skillCandidates.Results) != 0 || skillCandidates.Evaluated != 0 {
		t.Fatalf("Skill no-case analysis = %#v, err = %v", skillCandidates, err)
	}
}
