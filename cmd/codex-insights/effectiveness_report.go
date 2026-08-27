package main

import (
	"fmt"
	"sort"

	"codex-insights/internal/analyze"
	"codex-insights/internal/i18n"
)

type modelRoutingHypothesis struct {
	TaskType string
	Model    string
	Lower    analyze.ModelEffectiveness
	Other    analyze.ModelEffectiveness
}

func printEffectiveness(tr i18n.Translator, analysis analyze.EffectivenessAnalysis) {
	fmt.Println()
	fmt.Printf("%s:\n", tr.T("effectiveness"))
	if len(analysis.ModelComparisons) == 0 {
		fmt.Printf("  %s\n", tr.T("effectiveness_insufficient"))
	} else {
		currentType := ""
		for _, comparison := range analysis.ModelComparisons {
			if comparison.TaskType != currentType {
				currentType = comparison.TaskType
				fmt.Printf("  %s:\n", tr.TaskType(currentType))
			}
			fmt.Printf("    %s / %s — ", comparison.Model, comparison.Effort)
			printEffectivenessStats(tr, comparison.Stats)
		}
		fmt.Printf("  %s\n", tr.T("effectiveness_caveat"))
	}

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("subagent_effectiveness"))
	if len(analysis.SubagentGlobal) == 0 {
		fmt.Printf("  %s\n", tr.T("effectiveness_insufficient"))
	} else {
		for _, cohort := range analysis.SubagentGlobal {
			label := tr.T("without_subagents")
			if cohort.UsesSubagents {
				label = tr.T("with_subagents")
			}
			fmt.Printf("  %s — ", label)
			printEffectivenessStats(tr, cohort.Stats)
		}
		for _, comparison := range analysis.SubagentByTaskType {
			fmt.Printf("  %s:\n", tr.TaskType(comparison.TaskType))
			fmt.Printf("    %s — ", tr.T("without_subagents"))
			printEffectivenessStats(tr, comparison.Without.Stats)
			fmt.Printf("    %s — ", tr.T("with_subagents"))
			printEffectivenessStats(tr, comparison.WithSubagents.Stats)
		}
	}
	fmt.Printf("  %s\n", tr.T("subagent_selection_bias"))
}

func printEffectivenessStats(tr i18n.Translator, stats analyze.EffectivenessStats) {
	rate := 0.0
	if stats.Samples > 0 {
		rate = 100 * float64(stats.Steering) / float64(stats.Samples)
	}
	fmt.Printf(
		"%s=%d, %s=%.1f%%, %s=%.0f, %s=%.1f, %s=%.1f\n",
		tr.T("samples"), stats.Samples,
		tr.T("steering_rate"), rate,
		tr.T("average_tokens"), stats.AverageTokens,
		tr.T("average_seconds"), stats.AverageSeconds,
		tr.T("average_tool_calls"), stats.AverageToolCalls,
	)
}

func printHumanInsights(
	tr i18n.Translator,
	effectiveness analyze.EffectivenessAnalysis,
	steering []analyze.SteeringResult,
	reasons []analyze.SteeringReasonResult,
	prevention []analyze.PreventionResult,
	promptQuality []analyze.PromptQualityResult,
	agentsRules []analyze.AgentsRuleResult,
	skillCandidates []analyze.SkillCandidateResult,
	validation []analyze.ValidationResult,
) {
	fmt.Println()
	fmt.Printf("%s:\n", tr.T("insights_summary"))
	if effectiveness.Overall.Samples > 0 {
		fmt.Printf("  %s\n", insightText(tr, "summary", effectiveness.Overall.Samples, 100*steeringRate(effectiveness.Overall)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	fmt.Printf("%s:\n", tr.T("insights_strengths"))
	if strength := lowestMeaningfulTaskType(effectiveness.TaskTypeStats); strength != nil {
		fmt.Printf("  %s\n", insightText(tr, "strength", strength.Stats.Samples, 100*float64(strength.Stats.Steering)/float64(strength.Stats.Samples), tr.TaskType(strength.TaskType)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	fmt.Printf("%s:\n", tr.T("insights_weaknesses"))
	weaknessPrinted := false
	if weakness := highestMeaningfulTaskType(effectiveness.TaskTypeStats); weakness != nil {
		fmt.Printf("  %s\n", insightText(tr, "weakness", weakness.Stats.Samples, 100*float64(weakness.Stats.Steering)/float64(weakness.Stats.Samples), tr.TaskType(weakness.TaskType)))
		weaknessPrinted = true
	}
	if top := topReason(reasons); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "reason", tr.SteeringReason(top)))
		weaknessPrinted = true
	}
	if !weaknessPrinted {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	fmt.Printf("%s:\n", tr.T("insights_high"))
	if top := topValidation(validation); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "validation", tr.ValidationType(top)))
	} else if top := topReason(reasons); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "reason_recommendation", tr.SteeringReason(top)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	fmt.Printf("%s:\n", tr.T("insights_medium"))
	if top := topPromptQuality(promptQuality); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "prompt", tr.PromptQualityIssue(top)))
	} else if top := topAgentsRule(agentsRules); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "rule", tr.AgentsRule(top)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	fmt.Printf("%s:\n", tr.T("insights_low"))
	lowPrinted := false
	if len(skillCandidates) > 0 {
		stats := aggregateSkillCandidates(skillCandidates)
		if len(stats) > 0 {
			fmt.Printf("  %s\n", insightText(tr, "skill", tr.SkillCandidate(stats[0].Category)))
			lowPrinted = true
		}
	}
	if len(effectiveness.SubagentGlobal) == 2 {
		fmt.Printf("  %s\n", insightText(tr, "subagent_experiment"))
		lowPrinted = true
	}
	if hypothesis := matchedRoutingHypothesis(effectiveness.ModelComparisons); hypothesis != nil {
		fmt.Printf("  %s\n", insightText(tr, "model_routing", hypothesis.Lower.Model, hypothesis.Lower.Effort, tr.TaskType(hypothesis.TaskType), hypothesis.Other.Effort, 100*steeringRate(hypothesis.Lower.Stats), 100*steeringRate(hypothesis.Other.Stats)))
		lowPrinted = true
	}
	if !lowPrinted {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}
}

func insightText(tr i18n.Translator, kind string, values ...any) string {
	if tr.Language == i18n.Russian {
		switch kind {
		case "summary":
			return fmt.Sprintf("%d завершённых предыдущих задач с последующей поведенческой оценкой; корректировки: %.1f%%.", values[0], values[1])
		case "strength":
			return fmt.Sprintf("Тип задач «%s» имеет наименьшую долю корректировок среди значимых когорт: %.1f%% (%d наблюдений).", values[2], values[1], values[0])
		case "weakness":
			return fmt.Sprintf("Тип задач «%s» имеет наибольшую долю корректировок среди значимых когорт: %.1f%% (%d наблюдений).", values[2], values[1], values[0])
		case "reason":
			return fmt.Sprintf("Наиболее частая причина корректировок: %s.", values[0])
		case "validation":
			return fmt.Sprintf("Приоритетный эксперимент: добавить проверку «%s» к соответствующим задачам.", values[0])
		case "reason_recommendation":
			return fmt.Sprintf("Приоритетный эксперимент: отдельно проверять задачи с риском «%s» до завершения.", values[0])
		case "prompt":
			return fmt.Sprintf("Уточнять постановки, где чаще всего выявляется проблема «%s».", values[0])
		case "rule":
			return fmt.Sprintf("Рассмотреть правило проекта: %s", values[0])
		case "skill":
			return fmt.Sprintf("Проверить гипотезу о пользе переиспользуемого Skill: %s.", values[0])
		case "subagent_experiment":
			return "Провести контролируемый эксперимент с маршрутизацией задач к субагентам; текущие данные смещены по сложности."
		case "model_routing":
			return fmt.Sprintf("Для типа «%s» у модели %s когорта %s показывает %.1f%% корректировок против %.1f%% у %s при меньшей средней стоимости; проверить это как эксперимент, а не причинный вывод.", values[2], values[0], values[1], values[4], values[5], values[3])
		}
		return "Недостаточно данных."
	}
	switch kind {
	case "summary":
		return fmt.Sprintf("%d completed prior tasks had observable follow-up behavior labels; steering: %.1f%%.", values[0], values[1])
	case "strength":
		return fmt.Sprintf("%s has the lowest steering rate among meaningful cohorts: %.1f%% (%d samples).", values[2], values[1], values[0])
	case "weakness":
		return fmt.Sprintf("%s has the highest steering rate among meaningful cohorts: %.1f%% (%d samples).", values[2], values[1], values[0])
	case "reason":
		return fmt.Sprintf("Most frequent steering reason: %s.", values[0])
	case "validation":
		return fmt.Sprintf("Priority experiment: make %s part of the validation for applicable tasks.", values[0])
	case "reason_recommendation":
		return fmt.Sprintf("Priority experiment: explicitly check for %s risks before completion.", values[0])
	case "prompt":
		return fmt.Sprintf("Clarify prompts where %s is the most common issue.", values[0])
	case "rule":
		return fmt.Sprintf("Consider adopting this project rule: %s", values[0])
	case "skill":
		return fmt.Sprintf("Test the hypothesis that a reusable Skill would help: %s.", values[0])
	case "subagent_experiment":
		return "Run a controlled routing experiment with subagents; current data is confounded by task difficulty."
	case "model_routing":
		return fmt.Sprintf("For %s, model %s's %s cohort shows %.1f%% steering versus %.1f%% for %s with lower average cost; test this as an experiment, not a causal conclusion.", values[2], values[0], values[1], values[4], values[5], values[3])
	}
	return "Insufficient data."
}

func lowestMeaningfulTaskType(stats []analyze.TaskTypeEffectiveness) *analyze.TaskTypeEffectiveness {
	var result *analyze.TaskTypeEffectiveness
	for i := range stats {
		if !isActionableTaskType(stats[i].TaskType) ||
			stats[i].Stats.Samples < analyze.MinimumCohortSize {
			continue
		}
		if result == nil || steeringRate(stats[i].Stats) < steeringRate(result.Stats) {
			result = &stats[i]
		}
	}
	return result
}

func highestMeaningfulTaskType(stats []analyze.TaskTypeEffectiveness) *analyze.TaskTypeEffectiveness {
	var result *analyze.TaskTypeEffectiveness
	for i := range stats {
		if !isActionableTaskType(stats[i].TaskType) ||
			stats[i].Stats.Samples < analyze.MinimumCohortSize {
			continue
		}
		if result == nil || steeringRate(stats[i].Stats) > steeringRate(result.Stats) {
			result = &stats[i]
		}
	}
	return result
}

func isActionableTaskType(taskType string) bool {
	return taskType != "" && taskType != "unknown" && taskType != "other"
}

func matchedRoutingHypothesis(comparisons []analyze.ModelEffectiveness) *modelRoutingHypothesis {
	ordered := append([]analyze.ModelEffectiveness(nil), comparisons...)
	sort.Slice(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.TaskType != right.TaskType {
			return left.TaskType < right.TaskType
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		return left.Effort < right.Effort
	})
	for i := range ordered {
		for j := i + 1; j < len(ordered); j++ {
			left, right := ordered[i], ordered[j]
			if left.TaskType != right.TaskType || left.Model != right.Model {
				continue
			}
			if left.Stats.Samples < analyze.MinimumCohortSize || right.Stats.Samples < analyze.MinimumCohortSize {
				continue
			}
			lower, other := left, right
			if averageCost(right.Stats) < averageCost(left.Stats) {
				lower, other = right, left
			}
			if !lowerCost(lower.Stats, other.Stats) || steeringRate(lower.Stats) > steeringRate(other.Stats) {
				continue
			}
			return &modelRoutingHypothesis{TaskType: lower.TaskType, Model: lower.Model, Lower: lower, Other: other}
		}
	}
	return nil
}

func averageCost(stats analyze.EffectivenessStats) float64 {
	return stats.AverageTokens + 100*stats.AverageSeconds + 1000*stats.AverageToolCalls
}

func lowerCost(lower, other analyze.EffectivenessStats) bool {
	return lower.AverageTokens <= other.AverageTokens &&
		lower.AverageSeconds <= other.AverageSeconds &&
		lower.AverageToolCalls <= other.AverageToolCalls &&
		(lower.AverageTokens < other.AverageTokens ||
			lower.AverageSeconds < other.AverageSeconds ||
			lower.AverageToolCalls < other.AverageToolCalls)
}

func steeringRate(stats analyze.EffectivenessStats) float64 {
	if stats.Samples == 0 {
		return 0
	}
	return float64(stats.Steering) / float64(stats.Samples)
}

func topReason(results []analyze.SteeringReasonResult) string {
	return topString(results, func(r analyze.SteeringReasonResult) string { return r.Reason })
}
func topPromptQuality(results []analyze.PromptQualityResult) string {
	return topStringExcluding(results, func(r analyze.PromptQualityResult) string { return r.Issue }, "other")
}
func topAgentsRule(results []analyze.AgentsRuleResult) string {
	return topStringExcluding(results, func(r analyze.AgentsRuleResult) string { return r.Rule }, "other")
}
func topValidation(results []analyze.ValidationResult) string {
	return topStringExcluding(results, func(r analyze.ValidationResult) string { return r.ValidationType }, "other")
}

func topString[T any](results []T, key func(T) string) string {
	return topStringExcluding(results, key)
}

func topStringExcluding[T any](results []T, key func(T) string, excluded ...string) string {
	ignored := make(map[string]struct{}, len(excluded))
	for _, value := range excluded {
		ignored[value] = struct{}{}
	}
	counts := make(map[string]int)
	for _, result := range results {
		if value := key(result); value != "" {
			if _, skip := ignored[value]; skip {
				continue
			}
			counts[value]++
		}
	}
	values := make([]string, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] != counts[values[j]] {
			return counts[values[i]] > counts[values[j]]
		}
		return values[i] < values[j]
	})
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
