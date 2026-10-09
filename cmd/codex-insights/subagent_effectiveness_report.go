package main

import (
	"fmt"
	"strings"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func subagentEffectivenessLines(tr i18n.Translator, details *analyze.SubagentEffectivenessDetails) []string {
	if details == nil {
		return nil
	}
	lines := []string{tr.T("subeffect_population"), fmt.Sprintf("%s: %d / %d / %d; %s: %d",
		tr.T("subeffect_linkage"), details.LinkedTurns, details.UnlinkedTurns, details.ExcludedTurns, tr.T("subeffect_errors"), details.ReadErrors)}
	for _, cohort := range details.Cohorts {
		label := tr.T("subeffect_without")
		if cohort.UsesSubagents {
			label = tr.T("with_subagents")
		}
		lines = append(lines, subagentCohortLines(tr, cohort, label, subagentCohortComparable(cohort, details.Cohorts))...)
	}
	if len(details.Profiles) > 0 {
		lines = append(lines, tr.T("subeffect_profiles"))
		for _, cohort := range details.Profiles {
			label := tr.T("subeffect_"+cohort.Dimension) + " " + cohort.Name
			lines = append(lines, subagentCohortLines(tr, cohort, label, subagentCohortComparable(cohort, details.Profiles))...)
		}
	}
	if len(details.RoutingCohorts) > 0 {
		lines = append(lines, tr.T("subeffect_routing"))
		for _, cohort := range details.RoutingCohorts {
			label := tr.T("subeffect_without")
			if cohort.UsesSubagents {
				label = tr.T("with_subagents")
			}
			lines = append(lines, subagentCohortLines(tr, cohort, cohort.Name+" / "+label, subagentCohortComparable(cohort, details.RoutingCohorts))...)
		}
	}
	for _, key := range []string{"subeffect_sources", "subeffect_difficulty", "subeffect_outcome", "subagent_selection_bias"} {
		lines = append(lines, tr.T(key))
	}

	return lines
}

func subagentCohortComparable(cohort analyze.SubagentCohort, peers []analyze.SubagentCohort) bool {
	if cohort.Stats.Samples < analyze.MinimumCohortSize || cohort.TaskType == "unknown" {
		return false
	}
	for _, peer := range peers {
		if peer.TaskType != cohort.TaskType || peer.Dimension != cohort.Dimension || peer.Stats.Samples < analyze.MinimumCohortSize {
			continue
		}
		if cohort.Dimension == "routing" && peer.Name != cohort.Name {
			continue
		}
		if peer.Name != cohort.Name || peer.UsesSubagents != cohort.UsesSubagents {
			return true
		}
	}

	return false
}

func subagentCohortLines(tr i18n.Translator, cohort analyze.SubagentCohort, label string, comparable bool) []string {
	taskType := tr.T("subeffect_all")
	if cohort.TaskType != "" {
		taskType = tr.TaskType(cohort.TaskType)
	}
	rate := tr.T("subagent_resource_unknown")
	if cohort.Stats.Samples > 0 {
		rate = fmt.Sprintf("%.1f%%", percentageScale*steeringRate(cohort.Stats))
	}
	lines := []string{fmt.Sprintf("%s / %s — %s=%d, %s=%s", taskType, label, tr.T("samples"), cohort.Stats.Samples, tr.T("steering_rate"), rate)}
	if !comparable {
		lines = append(lines, "  "+tr.T("effectiveness_insufficient"))
	}
	lines = append(lines, "  "+tr.T("subeffect_parent")+": "+effectivenessResourceText(tr, cohort.Parent))
	if cohort.UsesSubagents {
		lines = append(lines, "  "+tr.T("subeffect_agents")+": "+effectivenessResourceText(tr, cohort.Agents))
	}

	return lines
}

func effectivenessResourceText(tr i18n.Translator, resources analyze.EffectivenessResources) string {
	parts := []string{}
	for _, row := range []struct {
		label  string
		metric analyze.ResourceMetric
	}{
		{"token_usage_record", resources.RecordedTokens}, {"token_count", resources.EstimatedTokens},
		{tr.T("average_seconds"), resources.Seconds}, {tr.T("average_tool_calls"), resources.Tools},
	} {
		value := tr.T("subagent_resource_unknown")
		if row.metric.Tasks > 0 {
			value = fmt.Sprintf("%.1f", row.metric.Average())
		}
		parts = append(parts, fmt.Sprintf("%s=%s (%s=%d; %s=%d/%d)", row.label, value,
			tr.T("samples"), row.metric.Tasks, tr.T("subeffect_coverage"), row.metric.KnownTurns, row.metric.MissingTurns))
	}
	parts = append(parts, fmt.Sprintf("%s=%d/%d/%d", tr.T("subeffect_counters"), resources.CounterMismatches, resources.UnverifiableCounters, resources.InvalidRecords))

	return strings.Join(parts, "; ")
}
