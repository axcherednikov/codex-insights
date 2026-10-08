package main

import (
	"fmt"
	"sort"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

type modelRoutingHypothesis struct {
	TaskType string
	Model    string
	Lower    analyze.ModelEffectiveness
	Other    analyze.ModelEffectiveness
}

func printEffectiveness(tr i18n.Translator, analysis analyze.EffectivenessAnalysis) {
	fmt.Println()
	printConsoleSection(tr.T("effectiveness"))
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
	printConsoleSection(tr.T("subagent_effectiveness"))
	if analysis.SubagentDetails != nil {
		for _, line := range subagentEffectivenessLines(tr, analysis.SubagentDetails) {
			fmt.Printf("  %s\n", line)
		}

		return
	}
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
		rate = percentageScale * float64(stats.Steering) / float64(stats.Samples)
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
	printConsoleSection(tr.T("insights_summary"))
	if effectiveness.Overall.Samples > 0 {
		fmt.Printf("  %s\n", insightText(tr, "summary", effectiveness.Overall.Samples, percentageScale*steeringRate(effectiveness.Overall)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	printConsoleSection(tr.T("insights_strengths"))
	if strength := lowestMeaningfulTaskType(effectiveness.TaskTypeStats); strength != nil {
		fmt.Printf("  %s\n", insightText(tr, "strength", strength.Stats.Samples, percentageScale*float64(strength.Stats.Steering)/float64(strength.Stats.Samples), tr.TaskType(strength.TaskType)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	printHumanPriorityInsights(tr, effectiveness, reasons, promptQuality, agentsRules, skillCandidates, validation)
}

func printHumanPriorityInsights(
	tr i18n.Translator,
	effectiveness analyze.EffectivenessAnalysis,
	reasons []analyze.SteeringReasonResult,
	promptQuality []analyze.PromptQualityResult,
	agentsRules []analyze.AgentsRuleResult,
	skillCandidates []analyze.SkillCandidateResult,
	validation []analyze.ValidationResult,
) {
	printConsoleSection(tr.T("insights_weaknesses"))
	weaknessPrinted := false
	if weakness := highestMeaningfulTaskType(effectiveness.TaskTypeStats); weakness != nil {
		fmt.Printf("  %s\n", insightText(tr, "weakness", weakness.Stats.Samples, percentageScale*float64(weakness.Stats.Steering)/float64(weakness.Stats.Samples), tr.TaskType(weakness.TaskType)))
		weaknessPrinted = true
	}
	if top := topReason(reasons); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "reason", tr.SteeringReason(top)))
		weaknessPrinted = true
	}
	if !weaknessPrinted {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	printConsoleSection(tr.T("insights_high"))
	if top := topValidation(validation); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "validation", tr.ValidationType(top)))
	} else if top := topReason(reasons); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "reason_recommendation", tr.SteeringReason(top)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	printConsoleSection(tr.T("insights_medium"))
	if top := topPromptQuality(promptQuality); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "prompt", tr.PromptQualityIssue(top)))
	} else if top := topAgentsRule(agentsRules); top != "" {
		fmt.Printf("  %s\n", insightText(tr, "rule", tr.AgentsRule(top)))
	} else {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}

	printConsoleSection(tr.T("insights_low"))
	lowPrinted := false
	if len(skillCandidates) > 0 {
		stats := aggregateSkillCandidates(skillCandidates)
		if len(stats) > 0 {
			fmt.Printf("  %s\n", insightText(tr, "skill", tr.SkillCandidate(stats[0].Category)))
			lowPrinted = true
		}
	}
	if len(effectiveness.SubagentGlobal) == subagentCohortCount {
		fmt.Printf("  %s\n", insightText(tr, "subagent_experiment"))
		lowPrinted = true
	}
	if hypothesis := matchedRoutingHypothesis(effectiveness.ModelComparisons); hypothesis != nil {
		fmt.Printf("  %s\n", insightText(tr, "model_routing", hypothesis.Lower.Model, hypothesis.Lower.Effort, tr.TaskType(hypothesis.TaskType), hypothesis.Other.Effort, percentageScale*steeringRate(hypothesis.Lower.Stats), percentageScale*steeringRate(hypothesis.Other.Stats)))
		lowPrinted = true
	}
	if !lowPrinted {
		fmt.Printf("  %s\n", tr.T("insights_limited"))
	}
}

type insightFormat struct {
	text      string
	arguments int
}

func insightText(tr i18n.Translator, kind string, values ...any) string {
	format, found := englishInsightFormat(kind)
	fallback := "Insufficient data."
	if tr.Language == i18n.Russian {
		format, found = russianInsightFormat(kind)
		fallback = "Недостаточно данных."
	}
	if !found || len(values) != format.arguments {
		return fallback
	}
	arguments := make([]any, len(values))
	for index, value := range values {
		if percentage, ok := value.(float64); ok {
			arguments[index] = fmt.Sprintf("%.1f", percentage)

			continue
		}
		arguments[index] = value
	}

	return fmt.Sprintf(format.text, arguments...)
}

func englishInsightFormat(kind string) (insightFormat, bool) {
	switch kind {
	case "summary":
		return insightFormat{"%[1]d completed prior tasks had observable follow-up behavior labels; steering: %[2]s%%.", 2}, true
	case "strength":
		return insightFormat{"%[3]s has the lowest steering rate among meaningful cohorts: %[2]s%% (%[1]d samples).", 3}, true
	case "weakness":
		return insightFormat{"%[3]s has the highest steering rate among meaningful cohorts: %[2]s%% (%[1]d samples).", 3}, true
	case "reason":
		return insightFormat{"Most frequent steering reason: %s.", 1}, true
	case "validation":
		return insightFormat{"Priority experiment: make %s part of the validation for applicable tasks.", 1}, true
	case "reason_recommendation":
		return insightFormat{"Priority experiment: explicitly check for %s risks before completion.", 1}, true
	case "prompt":
		return insightFormat{"Clarify prompts where %s is the most common issue.", 1}, true
	case "rule":
		return insightFormat{"Consider adopting this project rule: %s", 1}, true
	case "skill":
		return insightFormat{"Test the hypothesis that a reusable Skill would help: %s.", 1}, true
	case "subagent_experiment":
		return insightFormat{"Run a controlled routing experiment with subagents; current data is confounded by task difficulty.", 0}, true
	case "model_routing":
		return insightFormat{"For %[3]s, model %[1]s's %[2]s cohort shows %[5]s%% steering versus %[6]s%% for %[4]s with lower average cost; test this as an experiment, not a causal conclusion.", 6}, true
	default:
		return insightFormat{}, false
	}
}

func russianInsightFormat(kind string) (insightFormat, bool) {
	switch kind {
	case "summary":
		return insightFormat{"%[1]d завершённых предыдущих задач с последующей поведенческой оценкой; корректировки: %[2]s%%.", 2}, true
	case "strength":
		return insightFormat{"Тип задач «%[3]s» имеет наименьшую долю корректировок среди значимых когорт: %[2]s%% (%[1]d наблюдений).", 3}, true
	case "weakness":
		return insightFormat{"Тип задач «%[3]s» имеет наибольшую долю корректировок среди значимых когорт: %[2]s%% (%[1]d наблюдений).", 3}, true
	case "reason":
		return insightFormat{"Наиболее частая причина корректировок: %s.", 1}, true
	case "validation":
		return insightFormat{"Приоритетный эксперимент: добавить проверку «%s» к соответствующим задачам.", 1}, true
	case "reason_recommendation":
		return insightFormat{"Приоритетный эксперимент: отдельно проверять задачи с риском «%s» до завершения.", 1}, true
	case "prompt":
		return insightFormat{"Уточнять постановки, где чаще всего выявляется проблема «%s».", 1}, true
	case "rule":
		return insightFormat{"Рассмотреть правило проекта: %s", 1}, true
	case "skill":
		return insightFormat{"Проверить гипотезу о пользе переиспользуемого Skill: %s.", 1}, true
	case "subagent_experiment":
		return insightFormat{"Провести контролируемый эксперимент с маршрутизацией задач к субагентам; текущие данные смещены по сложности.", 0}, true
	case "model_routing":
		return insightFormat{"Для типа «%[3]s» у модели %[1]s когорта %[2]s показывает %[5]s%% корректировок против %[6]s%% у %[4]s при меньшей средней стоимости; проверить это как эксперимент, а не причинный вывод.", 6}, true
	default:
		return insightFormat{}, false
	}
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
