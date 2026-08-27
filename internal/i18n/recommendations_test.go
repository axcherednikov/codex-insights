package i18n

import "testing"

func TestRecommendationTranslationsDoNotLeakInternalKeys(t *testing.T) {
	ru, err := New("ru")
	if err != nil {
		t.Fatal(err)
	}
	if got := ru.AgentsRule("avoid_scope_creep"); got == "avoid_scope_creep" {
		t.Fatal("Russian AGENTS.md recommendation leaked internal key")
	}
	if got := ru.SkillCandidate("implementation_prompt_file"); got == "implementation_prompt_file" {
		t.Fatal("Russian Skill candidate leaked internal key")
	}
	if got := ru.PromptQualityIssue("missing_acceptance"); got == "missing_acceptance" {
		t.Fatal("Russian prompt quality issue leaked internal key")
	}
}

func TestEffectivenessAndInsightTranslationsAreLocalized(t *testing.T) {
	ru, err := New("ru")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"effectiveness", "subagent_effectiveness", "insights_summary",
		"insights_strengths", "insights_weaknesses", "insights_high",
		"insights_medium", "insights_low", "subagent_selection_bias",
	} {
		if got := ru.T(key); got == key || got == "" {
			t.Fatalf("Russian translation missing for %q: %q", key, got)
		}
	}
	if got := ru.TaskType("feature"); got == "feature" {
		t.Fatal("Russian task type leaked internal key")
	}
	if got := ru.TaskType("unknown"); got == "unknown" {
		t.Fatal("Russian unknown task type leaked internal key")
	}
}
