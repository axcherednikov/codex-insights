package golden

import (
	"fmt"
	"math"
	"sort"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/sessions"
)

const (
	HistoricalBefore              = "2026-08-26T14:56:26Z"
	HistoricalDays                = 0
	HistoricalLegacyOriginator    = "codex_exec"
	HistoricalSteeringTolerancePP = 1.5
	HistoricalCohortTolerancePP   = 8.0
	HistoricalMinimumCohort       = 10
)

type HistoricalOptions struct {
	Days                    int
	Before                  time.Time
	LegacyExcludeOriginator string
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
func (r GuardResult) Fail() bool { return r.Applicable && len(r.DeterministicMismatches) > 0 }

func CheckHistoricalGuard(options HistoricalOptions, actual DeterministicSnapshot, semantic SemanticSnapshot) GuardResult {
	result := GuardResult{Applicable: IsHistoricalInvocation(options)}
	if !result.Applicable {
		return result
	}
	expected := DeterministicSnapshot{Tasks: 1046, Complete: 1023, Aborted: 20, Incomplete: 3, Followups: 930, AverageTokens: 1652761, AverageSeconds: 258.3, AverageTools: 15.6}
	checks := []struct {
		name             string
		actual, expected float64
		rounded          bool
	}{
		{"tasks", float64(actual.Tasks), float64(expected.Tasks), false}, {"complete", float64(actual.Complete), float64(expected.Complete), false}, {"aborted", float64(actual.Aborted), float64(expected.Aborted), false}, {"incomplete", float64(actual.Incomplete), float64(expected.Incomplete), false}, {"followups", float64(actual.Followups), float64(expected.Followups), false},
		{"average tokens", actual.AverageTokens, expected.AverageTokens, true}, {"average seconds", actual.AverageSeconds, expected.AverageSeconds, true}, {"average tools", actual.AverageTools, expected.AverageTools, true},
	}
	for _, check := range checks {
		left, right := check.actual, check.expected
		if check.rounded {
			if check.name == "average seconds" || check.name == "average tools" {
				left = math.Round(left*10) / 10
				right = math.Round(right*10) / 10
			} else {
				left = math.Round(left)
				right = math.Round(right)
			}
		}
		if left != right {
			result.DeterministicMismatches = append(result.DeterministicMismatches, fmt.Sprintf("%s observed %.1f expected %.1f", check.name, left, right))
		}
	}
	if semantic.Samples != 930 {
		result.SemanticWarnings = append(result.SemanticWarnings, fmt.Sprintf("semantic sample count observed %d reference 930", semantic.Samples))
	}
	if math.Abs(semantic.SteeringRate-14.8) > HistoricalSteeringTolerancePP {
		result.SemanticWarnings = append(result.SemanticWarnings, fmt.Sprintf("overall steering observed %.1f%% reference 14.8%% tolerance +/-%.1fpp", semantic.SteeringRate, HistoricalSteeringTolerancePP))
	}
	for _, cohort := range []struct {
		name      string
		reference float64
	}{{"refactor", 30.0}, {"architecture", 20.6}} {
		observed, ok := semantic.Cohorts[cohort.name]
		if !ok {
			result.SemanticWarnings = append(result.SemanticWarnings, fmt.Sprintf("%s cohort missing (reference %.1f%%)", cohort.name, cohort.reference))
			continue
		}
		if observed.Samples < HistoricalMinimumCohort {
			result.SemanticWarnings = append(result.SemanticWarnings, fmt.Sprintf("%s cohort has %d samples below minimum %d", cohort.name, observed.Samples, HistoricalMinimumCohort))
			continue
		}
		if math.Abs(observed.SteeringRate-cohort.reference) > HistoricalCohortTolerancePP {
			result.SemanticWarnings = append(result.SemanticWarnings, fmt.Sprintf("%s cohort (%d samples) observed %.1f%% reference %.1f%% tolerance +/-%.1fpp", cohort.name, observed.Samples, observed.SteeringRate, cohort.reference, HistoricalCohortTolerancePP))
		}
	}
	sort.Strings(result.DeterministicMismatches)
	sort.Strings(result.SemanticWarnings)
	return result
}

func SemanticSnapshotFromResults(followups []sessions.Followup, results []analyze.SemanticResult) SemanticSnapshot {
	byID := make(map[string]analyze.SemanticResult, len(results))
	for _, result := range results {
		byID[result.TurnID] = result
	}
	steering, total := 0, 0
	cohorts := make(map[string]CohortRate)
	for _, followup := range followups {
		result, ok := byID[followup.PreviousTurnID]
		if !ok || result.FollowupLabel == "" {
			continue
		}
		total++
		if result.FollowupLabel == "steering" {
			steering++
		}
		cohort := cohorts[result.TaskType]
		cohort.Samples++
		if result.FollowupLabel == "steering" {
			cohort.SteeringRate++
		}
		cohorts[result.TaskType] = cohort
	}
	for key, value := range cohorts {
		if value.Samples > 0 {
			value.SteeringRate = value.SteeringRate / float64(value.Samples) * 100
			cohorts[key] = value
		}
	}
	rate := 0.0
	if total > 0 {
		rate = float64(steering) / float64(total) * 100
	}
	return SemanticSnapshot{Samples: total, SteeringRate: rate, Cohorts: cohorts}
}
