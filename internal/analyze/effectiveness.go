package analyze

import (
	"sort"

	"github.com/axcherednikov/codex-insights/internal/sessions"
)

// MinimumCohortSize is the smallest cohort from which this report will make a
// comparison. Smaller groups are retained in observations but not reported as
// evidence.
const (
	MinimumCohortSize     = 10
	millisecondsPerSecond = 1000
)

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
	SubagentDetails    *SubagentEffectivenessDetails
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
	evidence ...SubagentEvidence,
) EffectivenessAnalysis {
	observations := collectEffectivenessObservations(turns, taskTypes, steeringResults)
	if len(evidence) > 0 {
		for i := range observations {
			observations[i].usesSubagents = observations[i].usesSubagents || evidence[0].Tasks[observations[i].turnID].AgentTurns > 0
		}
	}
	taskTypeGroups, modelGroups := groupEffectivenessObservations(observations)
	analysis := EffectivenessAnalysis{
		Overall:            summarizeEffectiveness(observations),
		Observed:           len(observations),
		TaskTypeStats:      taskTypeEffectiveness(taskTypeGroups),
		ModelComparisons:   modelEffectiveness(modelGroups),
		SubagentGlobal:     globalSubagentEffectiveness(observations),
		SubagentByTaskType: taskTypeSubagentEffectiveness(observations),
	}
	if len(evidence) > 0 {
		analysis.SubagentDetails = summarizeSubagentEvidence(observations, evidence[0])
	}
	sortEffectiveness(&analysis)

	return analysis
}

// ResourceMetric retains observed coverage. A missing value never contributes
// a zero to an average; partial request totals remain explicitly partial.
type ResourceMetric struct {
	Total                           float64
	Tasks, KnownTurns, MissingTurns int
}

func (m ResourceMetric) Average() float64 {
	if m.Tasks == 0 {
		return 0
	}

	return m.Total / float64(m.Tasks)
}

type EffectivenessResources struct {
	RecordedTokens, EstimatedTokens, Seconds, Tools         ResourceMetric
	CounterMismatches, UnverifiableCounters, InvalidRecords int
}

func (r *EffectivenessResources) Add(other EffectivenessResources) {
	mergeEffectivenessResources(r, other, false)
}

type SubagentProfileEvidence struct {
	Dimension, Name string
	Resources       EffectivenessResources
}

type SubagentTaskEvidence struct {
	Parent, Agents EffectivenessResources
	AgentTurns     int
	Profiles       []SubagentProfileEvidence
}

// SubagentEvidence contains only resources attributed to selected user turns.
// Its map keys are join identities, never extra user-task observations.
type SubagentEvidence struct {
	Tasks                                                 map[string]SubagentTaskEvidence
	LinkedTurns, UnlinkedTurns, ExcludedTurns, ReadErrors int
}

type SubagentCohort struct {
	TaskType, Dimension, Name string
	UsesSubagents             bool
	Stats                     EffectivenessStats
	Parent, Agents            EffectivenessResources
}

type SubagentEffectivenessDetails struct {
	Cohorts                                               []SubagentCohort
	Profiles                                              []SubagentCohort
	RoutingCohorts                                        []SubagentCohort
	LinkedTurns, UnlinkedTurns, ExcludedTurns, ReadErrors int
}

func summarizeSubagentEvidence(observations []effectivenessObservation, evidence SubagentEvidence) *SubagentEffectivenessDetails {
	groups := map[string]*SubagentCohort{}
	profiles := map[string]*SubagentCohort{}
	routing := map[string]*SubagentCohort{}
	for _, observation := range observations {
		task := evidence.Tasks[observation.turnID]
		for _, taskType := range []string{"", observation.taskType} {
			evidenceCohort(groups, taskType+"\x00without", taskType, "", "", false)
			evidenceCohort(groups, taskType+"\x00with", taskType, "", "", true)
			key := taskType + "\x00without"
			if observation.usesSubagents {
				key = taskType + "\x00with"
			}
			cohort := evidenceCohort(groups, key, taskType, "", "", observation.usesSubagents)
			addEvidenceObservation(cohort, observation, task.Parent, task.Agents)
		}
		addProfileObservations(profiles, observation, task)
		addRoutingObservation(routing, observation, task)
	}

	return &SubagentEffectivenessDetails{
		Cohorts: sortedEvidenceCohorts(groups), Profiles: sortedEvidenceCohorts(profiles),
		RoutingCohorts: sortedEvidenceCohorts(routing),
		LinkedTurns:    evidence.LinkedTurns, UnlinkedTurns: evidence.UnlinkedTurns,
		ExcludedTurns: evidence.ExcludedTurns, ReadErrors: evidence.ReadErrors,
	}
}

func addRoutingObservation(groups map[string]*SubagentCohort, observation effectivenessObservation, task SubagentTaskEvidence) {
	if observation.model == "unknown" || observation.effort == "unknown" {
		return
	}
	name := observation.model + " / " + observation.effort
	baseKey := observation.taskType + "\x00" + name
	without := evidenceCohort(groups, baseKey+"\x00without", observation.taskType, "routing", name, false)
	with := evidenceCohort(groups, baseKey+"\x00with", observation.taskType, "routing", name, true)
	cohort := without
	if observation.usesSubagents {
		cohort = with
	}
	addEvidenceObservation(cohort, observation, task.Parent, task.Agents)
}

func evidenceCohort(groups map[string]*SubagentCohort, key, taskType, dimension, name string, uses bool) *SubagentCohort {
	if groups[key] == nil {
		groups[key] = &SubagentCohort{TaskType: taskType, Dimension: dimension, Name: name, UsesSubagents: uses}
	}

	return groups[key]
}

func addProfileObservations(groups map[string]*SubagentCohort, observation effectivenessObservation, task SubagentTaskEvidence) {
	// Multiple working turns and agents with the same profile still form one
	// user-task sample. Different profiles overlap and are not additive cohorts.
	resources := map[string]EffectivenessResources{}
	identities := map[string]SubagentProfileEvidence{}
	for _, profile := range task.Profiles {
		if profile.Name == "" || profile.Name == "unknown" {
			continue
		}
		key := profile.Dimension + "\x00" + profile.Name
		row := resources[key]
		mergeEffectivenessResources(&row, profile.Resources, false)
		resources[key], identities[key] = row, profile
	}
	for key, resource := range resources {
		identity := identities[key]
		cohort := evidenceCohort(groups, observation.taskType+"\x00"+key, observation.taskType, identity.Dimension, identity.Name, true)
		addEvidenceObservation(cohort, observation, task.Parent, resource)
	}
}

func addEvidenceObservation(cohort *SubagentCohort, observation effectivenessObservation, parent, agents EffectivenessResources) {
	cohort.Stats.Samples++
	if observation.steering {
		cohort.Stats.Steering++
	}
	mergeEffectivenessResources(&cohort.Parent, parent, true)
	mergeEffectivenessResources(&cohort.Agents, agents, true)
}

func mergeEffectivenessResources(target *EffectivenessResources, source EffectivenessResources, countTask bool) {
	pairs := [][2]*ResourceMetric{
		{&target.RecordedTokens, &source.RecordedTokens}, {&target.EstimatedTokens, &source.EstimatedTokens},
		{&target.Seconds, &source.Seconds}, {&target.Tools, &source.Tools},
	}
	for _, pair := range pairs {
		pair[0].Total += pair[1].Total
		pair[0].KnownTurns += pair[1].KnownTurns
		pair[0].MissingTurns += pair[1].MissingTurns
		if countTask && pair[1].KnownTurns > 0 {
			pair[0].Tasks++
		}
	}
	target.CounterMismatches += source.CounterMismatches
	target.UnverifiableCounters += source.UnverifiableCounters
	target.InvalidRecords += source.InvalidRecords
}

func sortedEvidenceCohorts(groups map[string]*SubagentCohort) []SubagentCohort {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]SubagentCohort, 0, len(keys))
	for _, key := range keys {
		result = append(result, *groups[key])
	}

	return result
}

func collectEffectivenessObservations(
	turns []sessions.Turn,
	taskTypes []TaskTypeResult,
	steeringResults []SteeringResult,
) []effectivenessObservation {
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
		observation, ok := effectivenessObservationForTurn(turn, typeByTurn, steeringByTurn, seen)
		if ok {
			observations = append(observations, observation)
		}
	}

	return observations
}

func effectivenessObservationForTurn(
	turn sessions.Turn,
	typeByTurn, steeringByTurn map[string]string,
	seen map[string]struct{},
) (effectivenessObservation, bool) {
	if turn.Status != "complete" || turn.ID == "" {
		return effectivenessObservation{}, false
	}
	if _, duplicate := seen[turn.ID]; duplicate {
		return effectivenessObservation{}, false
	}
	label, ok := steeringByTurn[turn.ID]
	if !ok {
		return effectivenessObservation{}, false
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
		// Historical cumulative counters can reset after compaction.
		tokens = 0
	}
	seen[turn.ID] = struct{}{}

	return effectivenessObservation{
		turnID:        turn.ID,
		taskType:      taskType,
		model:         model,
		effort:        effort,
		steering:      label == "steering",
		tokens:        tokens,
		durationMS:    turn.DurationMS,
		toolCalls:     turn.ToolCalls,
		usesSubagents: turn.UsesSubagents,
	}, true
}

func groupEffectivenessObservations(
	observations []effectivenessObservation,
) (map[string][]effectivenessObservation, map[string][]effectivenessObservation) {
	taskTypeGroups := make(map[string][]effectivenessObservation)
	modelGroups := make(map[string][]effectivenessObservation)
	for _, observation := range observations {
		modelKey := observation.taskType + "\x00" + observation.model + "\x00" + observation.effort
		modelGroups[modelKey] = append(modelGroups[modelKey], observation)
		taskTypeGroups[observation.taskType] = append(taskTypeGroups[observation.taskType], observation)
	}

	return taskTypeGroups, modelGroups
}

func taskTypeEffectiveness(groups map[string][]effectivenessObservation) []TaskTypeEffectiveness {
	stats := make([]TaskTypeEffectiveness, 0, len(groups))
	for taskType, observations := range groups {
		stats = append(stats, TaskTypeEffectiveness{TaskType: taskType, Stats: summarizeEffectiveness(observations)})
	}

	return stats
}

func modelEffectiveness(groups map[string][]effectivenessObservation) []ModelEffectiveness {
	qualifyingByType := make(map[string]int)
	for _, group := range groups {
		if len(group) >= MinimumCohortSize {
			qualifyingByType[group[0].taskType]++
		}
	}

	comparisons := make([]ModelEffectiveness, 0, len(groups))
	for _, group := range groups {
		if len(group) < MinimumCohortSize || qualifyingByType[group[0].taskType] < 2 {
			continue
		}
		comparisons = append(comparisons, ModelEffectiveness{
			TaskType: group[0].taskType,
			Model:    group[0].model,
			Effort:   group[0].effort,
			Stats:    summarizeEffectiveness(group),
		})
	}

	return comparisons
}

func globalSubagentEffectiveness(observations []effectivenessObservation) []SubagentEffectiveness {
	groups := make(map[bool][]effectivenessObservation)
	for _, observation := range observations {
		groups[observation.usesSubagents] = append(groups[observation.usesSubagents], observation)
	}
	if len(groups[true]) < MinimumCohortSize || len(groups[false]) < MinimumCohortSize {
		return nil
	}

	return []SubagentEffectiveness{
		{UsesSubagents: false, Stats: summarizeEffectiveness(groups[false])},
		{UsesSubagents: true, Stats: summarizeEffectiveness(groups[true])},
	}
}

func taskTypeSubagentEffectiveness(observations []effectivenessObservation) []SubagentTaskTypeEffectiveness {
	groupsByType := make(map[string]map[bool][]effectivenessObservation)
	for _, observation := range observations {
		groups := groupsByType[observation.taskType]
		if groups == nil {
			groups = make(map[bool][]effectivenessObservation)
			groupsByType[observation.taskType] = groups
		}
		groups[observation.usesSubagents] = append(groups[observation.usesSubagents], observation)
	}

	comparisons := make([]SubagentTaskTypeEffectiveness, 0, len(groupsByType))
	for taskType, groups := range groupsByType {
		comparison, ok := subagentTaskTypeComparison(taskType, groups)
		if ok {
			comparisons = append(comparisons, comparison)
		}
	}

	return comparisons
}

func subagentTaskTypeComparison(
	taskType string,
	groups map[bool][]effectivenessObservation,
) (SubagentTaskTypeEffectiveness, bool) {
	if len(groups[true]) < MinimumCohortSize || len(groups[false]) < MinimumCohortSize {
		return SubagentTaskTypeEffectiveness{}, false
	}

	return SubagentTaskTypeEffectiveness{
		TaskType: taskType,
		WithSubagents: SubagentEffectiveness{
			UsesSubagents: true,
			Stats:         summarizeEffectiveness(groups[true]),
		},
		Without: SubagentEffectiveness{
			UsesSubagents: false,
			Stats:         summarizeEffectiveness(groups[false]),
		},
	}, true
}

func sortEffectiveness(analysis *EffectivenessAnalysis) {
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
}

func summarizeEffectiveness(observations []effectivenessObservation) EffectivenessStats {
	var stats EffectivenessStats
	for _, observation := range observations {
		stats.Samples++
		if observation.steering {
			stats.Steering++
		}
		stats.AverageTokens += float64(observation.tokens)
		stats.AverageSeconds += float64(observation.durationMS) / millisecondsPerSecond
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
