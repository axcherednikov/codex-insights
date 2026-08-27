package analyze

import (
	"sort"

	"codex-insights/internal/sessions"
)

// MinimumCohortSize is the smallest cohort from which this report will make a
// comparison. Smaller groups are retained in observations but not reported as
// evidence.
const MinimumCohortSize = 10

type EffectivenessStats struct {
	Samples          int
	Steering         int
	AverageTokens    float64
	AverageSeconds   float64
	AverageToolCalls float64
}

type ModelEffectiveness struct {
	TaskType string
	Model    string
	Effort   string
	Stats    EffectivenessStats
}

type TaskTypeEffectiveness struct {
	TaskType string
	Stats    EffectivenessStats
}

type SubagentEffectiveness struct {
	UsesSubagents bool
	Stats         EffectivenessStats
}

type SubagentTaskTypeEffectiveness struct {
	TaskType      string
	WithSubagents SubagentEffectiveness
	Without       SubagentEffectiveness
}

type EffectivenessAnalysis struct {
	Overall            EffectivenessStats
	TaskTypeStats      []TaskTypeEffectiveness
	ModelComparisons   []ModelEffectiveness
	SubagentGlobal     []SubagentEffectiveness
	SubagentByTaskType []SubagentTaskTypeEffectiveness
	Observed           int
}

type effectivenessObservation struct {
	turnID        string
	taskType      string
	model         string
	effort        string
	steering      bool
	tokens        int64
	durationMS    int64
	toolCalls     int
	usesSubagents bool
}

// AggregateEffectiveness joins completed prior turns to their task type and
// steering labels by turn ID. A turn without a subsequent steering label is
// deliberately excluded: it cannot provide an observable behavior outcome.
func AggregateEffectiveness(
	turns []sessions.Turn,
	taskTypes []TaskTypeResult,
	steeringResults []SteeringResult,
) EffectivenessAnalysis {
	typeByTurn := make(map[string]string, len(taskTypes))
	for _, result := range taskTypes {
		typeByTurn[result.TurnID] = result.Type
	}
	steeringByTurn := make(map[string]string, len(steeringResults))
	for _, result := range steeringResults {
		steeringByTurn[result.PreviousTurnID] = result.Label
	}

	observations := make([]effectivenessObservation, 0, len(turns))
	seen := make(map[string]struct{}, len(turns))
	for _, turn := range turns {
		if turn.Status != "complete" || turn.ID == "" {
			continue
		}
		if _, ok := seen[turn.ID]; ok {
			continue
		}
		label, ok := steeringByTurn[turn.ID]
		if !ok {
			continue
		}
		taskType := typeByTurn[turn.ID]
		if taskType == "" {
			taskType = "unknown"
		}
		model := turn.Model
		if model == "" {
			model = "unknown"
		}
		effort := turn.Effort
		if effort == "" {
			effort = "unknown"
		}
		tokens := turn.Tokens
		if tokens < 0 {
			// Historical cumulative counters can reset after compaction. Do not
			// let an invalid negative delta produce impossible report averages.
			tokens = 0
		}
		seen[turn.ID] = struct{}{}
		observations = append(observations, effectivenessObservation{
			turnID:        turn.ID,
			taskType:      taskType,
			model:         model,
			effort:        effort,
			steering:      label == "steering",
			tokens:        tokens,
			durationMS:    turn.DurationMS,
			toolCalls:     turn.ToolCalls,
			usesSubagents: turn.UsesSubagents,
		})
	}

	analysis := EffectivenessAnalysis{
		Overall:  summarizeEffectiveness(observations),
		Observed: len(observations),
	}
	modelGroups := make(map[string][]effectivenessObservation)
	taskTypeGroups := make(map[string][]effectivenessObservation)
	for _, observation := range observations {
		key := observation.taskType + "\x00" + observation.model + "\x00" + observation.effort
		modelGroups[key] = append(modelGroups[key], observation)
		taskTypeGroups[observation.taskType] = append(taskTypeGroups[observation.taskType], observation)
	}
	for taskType, group := range taskTypeGroups {
		analysis.TaskTypeStats = append(analysis.TaskTypeStats, TaskTypeEffectiveness{TaskType: taskType, Stats: summarizeEffectiveness(group)})
	}
	qualifyingByType := make(map[string]int)
	for _, group := range modelGroups {
		if len(group) >= MinimumCohortSize {
			qualifyingByType[group[0].taskType]++
		}
	}
	for _, group := range modelGroups {
		if len(group) < MinimumCohortSize || qualifyingByType[group[0].taskType] < 2 {
			continue
		}
		analysis.ModelComparisons = append(analysis.ModelComparisons, ModelEffectiveness{
			TaskType: group[0].taskType,
			Model:    group[0].model,
			Effort:   group[0].effort,
			Stats:    summarizeEffectiveness(group),
		})
	}

	subagentGroups := make(map[bool][]effectivenessObservation)
	for _, observation := range observations {
		subagentGroups[observation.usesSubagents] = append(subagentGroups[observation.usesSubagents], observation)
	}
	if len(subagentGroups[true]) >= MinimumCohortSize && len(subagentGroups[false]) >= MinimumCohortSize {
		analysis.SubagentGlobal = []SubagentEffectiveness{
			{UsesSubagents: false, Stats: summarizeEffectiveness(subagentGroups[false])},
			{UsesSubagents: true, Stats: summarizeEffectiveness(subagentGroups[true])},
		}
	}

	subagentByType := make(map[string]map[bool][]effectivenessObservation)
	for _, observation := range observations {
		groups := subagentByType[observation.taskType]
		if groups == nil {
			groups = make(map[bool][]effectivenessObservation)
			subagentByType[observation.taskType] = groups
		}
		groups[observation.usesSubagents] = append(groups[observation.usesSubagents], observation)
	}
	for taskType, groups := range subagentByType {
		if len(groups[true]) < MinimumCohortSize || len(groups[false]) < MinimumCohortSize {
			continue
		}
		analysis.SubagentByTaskType = append(analysis.SubagentByTaskType, SubagentTaskTypeEffectiveness{
			TaskType: taskType,
			WithSubagents: SubagentEffectiveness{
				UsesSubagents: true,
				Stats:         summarizeEffectiveness(groups[true]),
			},
			Without: SubagentEffectiveness{
				UsesSubagents: false,
				Stats:         summarizeEffectiveness(groups[false]),
			},
		})
	}

	sort.Slice(analysis.ModelComparisons, func(i, j int) bool {
		left, right := analysis.ModelComparisons[i], analysis.ModelComparisons[j]
		if left.TaskType != right.TaskType {
			return left.TaskType < right.TaskType
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		return left.Effort < right.Effort
	})
	sort.Slice(analysis.TaskTypeStats, func(i, j int) bool {
		left, right := analysis.TaskTypeStats[i], analysis.TaskTypeStats[j]
		leftRate := float64(left.Stats.Steering) / float64(left.Stats.Samples)
		rightRate := float64(right.Stats.Steering) / float64(right.Stats.Samples)
		if leftRate != rightRate {
			return leftRate > rightRate
		}
		return left.TaskType < right.TaskType
	})
	sort.Slice(analysis.SubagentByTaskType, func(i, j int) bool {
		return analysis.SubagentByTaskType[i].TaskType < analysis.SubagentByTaskType[j].TaskType
	})
	return analysis
}

func summarizeEffectiveness(observations []effectivenessObservation) EffectivenessStats {
	var stats EffectivenessStats
	for _, observation := range observations {
		stats.Samples++
		if observation.steering {
			stats.Steering++
		}
		stats.AverageTokens += float64(observation.tokens)
		stats.AverageSeconds += float64(observation.durationMS) / 1000
		stats.AverageToolCalls += float64(observation.toolCalls)
	}
	if stats.Samples > 0 {
		count := float64(stats.Samples)
		stats.AverageTokens /= count
		stats.AverageSeconds /= count
		stats.AverageToolCalls /= count
	}
	return stats
}
