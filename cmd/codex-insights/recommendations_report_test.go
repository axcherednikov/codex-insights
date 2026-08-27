package main

import (
	"testing"

	"codex-insights/internal/analyze"
)

func TestAggregateRecommendationsAreDeterministic(t *testing.T) {
	prompt := aggregatePromptQuality([]analyze.PromptQualityResult{
		{Issue: "missing_context"}, {Issue: "missing_constraints"}, {Issue: "missing_context"},
	})
	if len(prompt) != 2 || prompt[0].Issue != "missing_context" || prompt[0].Count != 2 {
		t.Fatalf("unexpected prompt quality stats: %#v", prompt)
	}

	agents := aggregateAgentsRules([]analyze.AgentsRuleResult{
		{Rule: "other"}, {Rule: "avoid_scope_creep"}, {Rule: "avoid_scope_creep"}, {Rule: "preserve_project_architecture"},
	})
	if len(agents) != 2 || agents[0].Rule != "avoid_scope_creep" || agents[0].Count != 2 {
		t.Fatalf("unexpected AGENTS.md stats: %#v", agents)
	}
}

func TestAggregateSkillCandidatesRequiresTwoSupportingCases(t *testing.T) {
	stats := aggregateSkillCandidates([]analyze.SkillCandidateResult{
		{Category: "implementation_prompt_file"},
		{Category: "bounded_architecture_review"},
		{Category: "bounded_architecture_review"},
		{Category: "other"},
	})
	if len(stats) != 1 || stats[0].Category != "bounded_architecture_review" || stats[0].Count != 2 {
		t.Fatalf("unexpected Skill candidate stats: %#v", stats)
	}
}
