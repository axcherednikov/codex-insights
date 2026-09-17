package main

import (
	"fmt"
	"sort"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

type promptQualityStat struct {
	Issue string
	Count int
}

type agentsRuleStat struct {
	Rule  string
	Count int
}

type skillCandidateStat struct {
	Category string
	Count    int
}

func aggregatePromptQuality(results []analyze.PromptQualityResult) []promptQualityStat {
	counts := make(map[string]int)
	for _, result := range results {
		counts[result.Issue]++
	}
	stats := make([]promptQualityStat, 0, len(counts))
	for issue, count := range counts {
		stats = append(stats, promptQualityStat{Issue: issue, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Issue < stats[j].Issue
	})
	return stats
}

func aggregateAgentsRules(results []analyze.AgentsRuleResult) []agentsRuleStat {
	counts := make(map[string]int)
	for _, result := range results {
		if result.Rule != "other" {
			counts[result.Rule]++
		}
	}
	stats := make([]agentsRuleStat, 0, len(counts))
	for rule, count := range counts {
		stats = append(stats, agentsRuleStat{Rule: rule, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Rule < stats[j].Rule
	})
	return stats
}

func aggregateSkillCandidates(results []analyze.SkillCandidateResult) []skillCandidateStat {
	counts := make(map[string]int)
	for _, result := range results {
		if result.Category != "other" {
			counts[result.Category]++
		}
	}
	stats := make([]skillCandidateStat, 0, len(counts))
	for category, count := range counts {
		if count >= 2 {
			stats = append(stats, skillCandidateStat{Category: category, Count: count})
		}
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Category < stats[j].Category
	})
	return stats
}

func printPromptQuality(tr i18n.Translator, results []analyze.PromptQualityResult) {
	stats := aggregatePromptQuality(results)
	if len(stats) == 0 {
		return
	}
	fmt.Println()
	printConsoleSection(tr.T("prompt_quality"))
	for _, stat := range stats {
		fmt.Printf("  %-34s %4d\n", tr.PromptQualityIssue(stat.Issue), stat.Count)
	}
}

func printAgentsRecommendations(tr i18n.Translator, results []analyze.AgentsRuleResult) {
	stats := aggregateAgentsRules(results)
	if len(stats) == 0 {
		return
	}
	fmt.Println()
	printConsoleSection(tr.T("agents_recommendations"))
	for _, stat := range stats {
		fmt.Printf("  %-4d %s\n", stat.Count, tr.AgentsRule(stat.Rule))
	}
}

func printSkillCandidates(tr i18n.Translator, results []analyze.SkillCandidateResult) {
	stats := aggregateSkillCandidates(results)
	if len(stats) == 0 {
		return
	}
	fmt.Println()
	printConsoleSection(tr.T("skill_candidates"))
	for _, stat := range stats {
		fmt.Printf("  %s (%d %s)\n", tr.SkillCandidate(stat.Category), stat.Count, tr.T("supporting_cases"))
		fmt.Printf("    %s\n", tr.SkillCandidatePurpose(stat.Category))
		fmt.Printf("    %s: %s\n", tr.T("usefulness"), tr.SkillCandidateUsefulness(stat.Category))
	}
}
