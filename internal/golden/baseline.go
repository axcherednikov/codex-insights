package golden

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

const (
	HistoricalBefore              = "2026-08-26T14:56:26Z"
	HistoricalDays                = 0
	HistoricalLegacyOriginator    = "codex_exec"
	HistoricalSteeringTolerancePP = 1.5
	HistoricalCohortTolerancePP   = 8.0
	HistoricalMinimumCohort       = 10
	HistoricalMethodology         = "semantic-v1"

	historicalExpectedTasks          = 1046
	historicalExpectedComplete       = 1023
	historicalExpectedAborted        = 20
	historicalExpectedIncomplete     = 3
	historicalExpectedFollowups      = 930
	historicalExpectedAverageTokens  = 1652761
	historicalExpectedAvgSeconds     = 258.3
	historicalExpectedAvgTools       = 15.6
	historicalExpectedSemantic       = 930
	historicalExpectedSteeringRate   = 14.8
	historicalRefactorRate           = 30.0
	historicalArchitectureRate       = 20.6
	historicalPercent                = 100.0
	historicalRoundingFactor         = 10.0
	historicalIntegerRoundFactor     = 1.0
	historicalOverallWarningCapacity = 2
)

type HistoricalOptions struct {
	Days                    int
	Before                  time.Time
	LegacyExcludeOriginator string
	MethodologyVersion      string
}

func IsHistoricalInvocation(options HistoricalOptions) bool {
	before, err := time.Parse(time.RFC3339, HistoricalBefore)

	return err == nil && options.Days == HistoricalDays && options.Before.Equal(before) && options.LegacyExcludeOriginator == HistoricalLegacyOriginator
}

type DeterministicSnapshot struct {
	Tasks          int
	Complete       int
	Aborted        int
	Incomplete     int
	Followups      int
	AverageTokens  float64
	AverageSeconds float64
	AverageTools   float64
}

type SemanticSnapshot struct {
	Samples      int
	SteeringRate float64
	Cohorts      map[string]CohortRate
}

type CohortRate struct {
	Samples      int
	SteeringRate float64
}

type GuardResult struct {
	Applicable              bool
	DeterministicMismatches []string
	SemanticWarnings        []string
}

func (r GuardResult) Pass() bool {
	return r.Applicable && len(r.DeterministicMismatches) == 0 && len(r.SemanticWarnings) == 0
}

func (r GuardResult) Fail() bool {
	return r.Applicable && len(r.DeterministicMismatches) > 0
}

func CheckHistoricalGuard(options HistoricalOptions, actual DeterministicSnapshot, semantic SemanticSnapshot) GuardResult {
	result := GuardResult{Applicable: IsHistoricalInvocation(options)}
	if !result.Applicable {
		return result
	}
	result.DeterministicMismatches = historicalDeterministicMismatches(options, actual)
	result.SemanticWarnings = historicalSemanticWarnings(options, semantic)
	sort.Strings(result.DeterministicMismatches)
	sort.Strings(result.SemanticWarnings)

	return result
}

func historicalDeterministicMismatches(options HistoricalOptions, actual DeterministicSnapshot) []string {
	expected := historicalExpectedSnapshot()
	checks := []struct {
		name             string
		actual, expected float64
		precision        float64
	}{
		{"tasks", float64(actual.Tasks), float64(expected.Tasks), historicalIntegerRoundFactor},
		{"complete", float64(actual.Complete), float64(expected.Complete), historicalIntegerRoundFactor},
		{"aborted", float64(actual.Aborted), float64(expected.Aborted), historicalIntegerRoundFactor},
		{"incomplete", float64(actual.Incomplete), float64(expected.Incomplete), historicalIntegerRoundFactor},
		{"average tokens", actual.AverageTokens, expected.AverageTokens, historicalIntegerRoundFactor},
		{"average seconds", actual.AverageSeconds, expected.AverageSeconds, historicalRoundingFactor},
		{"average tools", actual.AverageTools, expected.AverageTools, historicalRoundingFactor},
	}
	if options.MethodologyVersion == HistoricalMethodology {
		checks = append(checks, struct {
			name             string
			actual, expected float64
			precision        float64
		}{"followups", float64(actual.Followups), float64(expected.Followups), historicalIntegerRoundFactor})
	}

	return compareHistoricalChecks(checks)
}

func historicalExpectedSnapshot() DeterministicSnapshot {
	return DeterministicSnapshot{
		Tasks:          historicalExpectedTasks,
		Complete:       historicalExpectedComplete,
		Aborted:        historicalExpectedAborted,
		Incomplete:     historicalExpectedIncomplete,
		Followups:      historicalExpectedFollowups,
		AverageTokens:  historicalExpectedAverageTokens,
		AverageSeconds: historicalExpectedAvgSeconds,
		AverageTools:   historicalExpectedAvgTools,
	}
}

func compareHistoricalChecks(checks []struct {
	name             string
	actual, expected float64
	precision        float64
}) []string {
	mismatches := make([]string, 0, len(checks))
	for _, check := range checks {
		actual := roundedHistoricalValue(check.actual, check.precision)
		expected := roundedHistoricalValue(check.expected, check.precision)
		if actual != expected {
			mismatches = append(mismatches, fmt.Sprintf("%s observed %.1f expected %.1f", check.name, actual, expected))
		}
	}

	return mismatches
}

func roundedHistoricalValue(value, precision float64) float64 {
	return math.Round(value*precision) / precision
}

func historicalSemanticWarnings(options HistoricalOptions, semantic SemanticSnapshot) []string {
	if options.MethodologyVersion != HistoricalMethodology {
		current := options.MethodologyVersion
		if current == "" {
			current = "unknown"
		}

		return []string{fmt.Sprintf("historical reference methodology %s; current methodology %s; follow-up and semantic metrics require calibration", HistoricalMethodology, current)}
	}
	warnings := historicalOverallWarnings(semantic)

	return append(warnings, historicalCohortWarnings(semantic)...)
}

func historicalOverallWarnings(semantic SemanticSnapshot) []string {
	warnings := make([]string, 0, historicalOverallWarningCapacity)
	if semantic.Samples != historicalExpectedSemantic {
		warnings = append(warnings, fmt.Sprintf("semantic sample count observed %d reference 930", semantic.Samples))
	}
	if math.Abs(semantic.SteeringRate-historicalExpectedSteeringRate) > HistoricalSteeringTolerancePP {
		warnings = append(warnings, fmt.Sprintf("overall steering observed %.1f%% reference 14.8%% tolerance +/-%.1fpp", semantic.SteeringRate, HistoricalSteeringTolerancePP))
	}

	return warnings
}

func historicalCohortWarnings(semantic SemanticSnapshot) []string {
	cohorts := []struct {
		name      string
		reference float64
	}{
		{"refactor", historicalRefactorRate},
		{"architecture", historicalArchitectureRate},
	}
	warnings := make([]string, 0, len(cohorts))
	for _, cohort := range cohorts {
		warning := historicalCohortWarning(semantic.Cohorts, cohort.name, cohort.reference)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}

	return warnings
}

func historicalCohortWarning(cohorts map[string]CohortRate, name string, reference float64) string {
	observed, ok := cohorts[name]
	if !ok {
		return fmt.Sprintf("%s cohort missing (reference %.1f%%)", name, reference)
	}
	if observed.Samples < HistoricalMinimumCohort {
		return fmt.Sprintf("%s cohort has %d samples below minimum %d", name, observed.Samples, HistoricalMinimumCohort)
	}
	if math.Abs(observed.SteeringRate-reference) > HistoricalCohortTolerancePP {
		return fmt.Sprintf("%s cohort (%d samples) observed %.1f%% reference %.1f%% tolerance +/-%.1fpp", name, observed.Samples, observed.SteeringRate, reference, HistoricalCohortTolerancePP)
	}

	return ""
}

func SemanticSnapshotFromResults(followups []sessions.Followup, results []analyze.SemanticResult) SemanticSnapshot {
	byID := semanticResultsByTurnID(results)
	cohorts, steering, total := countLabeledFollowups(followups, byID)
	finalizeCohortRates(cohorts)

	return SemanticSnapshot{Samples: total, SteeringRate: percentage(steering, total), Cohorts: cohorts}
}

func semanticResultsByTurnID(results []analyze.SemanticResult) map[string]analyze.SemanticResult {
	byID := make(map[string]analyze.SemanticResult, len(results))
	for _, result := range results {
		byID[result.TurnID] = result
	}

	return byID
}

func countLabeledFollowups(followups []sessions.Followup, byID map[string]analyze.SemanticResult) (map[string]CohortRate, int, int) {
	cohorts := make(map[string]CohortRate)
	steering, total := 0, 0
	for _, followup := range followups {
		result, ok := byID[followup.PreviousTurnID]
		if !ok || result.FollowupLabel == "" {
			continue
		}
		total++
		cohort := cohorts[result.TaskType]
		cohort.Samples++
		if result.FollowupLabel == "steering" {
			steering++
			cohort.SteeringRate++
		}
		cohorts[result.TaskType] = cohort
	}

	return cohorts, steering, total
}

func finalizeCohortRates(cohorts map[string]CohortRate) {
	for key, value := range cohorts {
		value.SteeringRate = percentage(int(value.SteeringRate), value.Samples)
		cohorts[key] = value
	}
}

func percentage(part, total int) float64 {
	if total == 0 {
		return 0
	}

	return float64(part) / float64(total) * historicalPercent
}
