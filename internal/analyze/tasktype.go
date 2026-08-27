package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	analysiscache "codex-insights/internal/cache"
	"codex-insights/internal/judge"
	"codex-insights/internal/sessions"
)

const taskTypeCacheVersion = "task-type-v1"

type TaskTypeResult struct {
	TurnID     string  `json:"turn_id"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

type TaskTypeAnalysis struct {
	Results   []TaskTypeResult
	CacheHits int
	Evaluated int
}

type taskTypeResponse struct {
	Results []TaskTypeResult `json:"results"`
}

type TaskTypeAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewTaskTypeAnalyzer() (TaskTypeAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return TaskTypeAnalyzer{}, err
	}

	return TaskTypeAnalyzer{
		runner:    judge.New(),
		cache:     store,
		batchSize: 25,
	}, nil
}

func (a TaskTypeAnalyzer) Analyze(
	interactions []sessions.Interaction,
) (TaskTypeAnalysis, error) {
	tasks := make([]sessions.Interaction, 0, len(interactions))

	for _, interaction := range interactions {
		if interaction.Prompt == "" {
			continue
		}

		tasks = append(tasks, interaction)
	}

	analysis := TaskTypeAnalysis{
		Results: make([]TaskTypeResult, 0, len(tasks)),
	}

	resultsByTurn := make(
		map[string]TaskTypeResult,
		len(tasks),
	)

	pending := make([]sessions.Interaction, 0, len(tasks))

	for _, task := range tasks {
		var cached TaskTypeResult

		if a.cache != nil &&
			a.cache.Get(taskTypeCacheKey(task), &cached) &&
			cached.TurnID == task.TurnID {

			resultsByTurn[task.TurnID] = cached
			analysis.CacheHits++
			continue
		}

		pending = append(pending, task)
	}

	for start := 0; start < len(pending); start += a.batchSize {
		end := start + a.batchSize
		if end > len(pending) {
			end = len(pending)
		}

		batch := pending[start:end]

		batchResults, err := analyzeBatchWithSplit(batch, a.analyzeBatch)
		if err != nil {
			return TaskTypeAnalysis{}, fmt.Errorf(
				"analyze task type batch %d-%d: %w",
				start,
				end,
				err,
			)
		}

		analysis.Evaluated += len(batch)

		for _, result := range batchResults {
			resultsByTurn[result.TurnID] = result
		}

		if a.cache != nil {
			for _, task := range batch {
				result, ok := resultsByTurn[task.TurnID]
				if !ok {
					continue
				}

				if err := a.cache.Set(
					taskTypeCacheKey(task),
					result,
				); err != nil {
					return TaskTypeAnalysis{}, err
				}
			}

			if err := a.cache.Save(); err != nil {
				return TaskTypeAnalysis{}, err
			}
		}
	}

	for _, task := range tasks {
		result, ok := resultsByTurn[task.TurnID]
		if !ok {
			return TaskTypeAnalysis{}, fmt.Errorf(
				"missing task type result for turn %q",
				task.TurnID,
			)
		}

		analysis.Results = append(analysis.Results, result)
	}

	return analysis, nil
}

func (a TaskTypeAnalyzer) analyzeBatch(
	tasks []sessions.Interaction,
) ([]TaskTypeResult, error) {
	type record struct {
		TurnID string `json:"turn_id"`
		Prompt string `json:"prompt"`
	}

	records := make([]record, 0, len(tasks))

	for _, task := range tasks {
		records = append(records, record{
			TurnID: task.TurnID,
			Prompt: task.Prompt,
		})
	}

	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal tasks: %w", err)
	}

	prompt := `Classify every task into exactly one category.

Categories:
bugfix = finding or fixing incorrect existing behavior
feature = implementing new functionality
refactor = restructuring existing code without primarily adding behavior
tests = creating, fixing, or improving tests or benchmarks
code_review = reviewing code, PR, MR, diff, or implementation
architecture = system design, architecture decisions, or decomposition
devops = Docker, Kubernetes, CI/CD, deployment, infrastructure, or monitoring
research = investigation, explanation, comparison, or technical learning
documentation = writing or editing docs, README, task descriptions, or artifacts
other = none of the above

Use the primary intent of the user prompt.
Return every record exactly once.
Do not use tools.

Tasks:
` + string(data)

	var response taskTypeResponse

	if err := a.runner.Run(
		prompt,
		taskTypeSchema(),
		&response,
	); err != nil {
		return nil, err
	}

	if err := validateTaskTypeResults(
		tasks,
		response.Results,
	); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func validateTaskTypeResults(
	tasks []sessions.Interaction,
	results []TaskTypeResult,
) error {
	expected := make(map[string]struct{}, len(tasks))

	for _, task := range tasks {
		expected[task.TurnID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(results))

	for _, result := range results {
		if _, ok := expected[result.TurnID]; !ok {
			return fmt.Errorf(
				"judge returned unexpected turn id %q",
				result.TurnID,
			)
		}

		if _, duplicate := seen[result.TurnID]; duplicate {
			return fmt.Errorf(
				"judge returned duplicate turn id %q",
				result.TurnID,
			)
		}

		if !isTaskType(result.Type) {
			return fmt.Errorf(
				"judge returned unsupported task type %q",
				result.Type,
			)
		}

		seen[result.TurnID] = struct{}{}
	}

	if len(seen) != len(expected) {
		return fmt.Errorf(
			"judge returned %d results, expected %d",
			len(seen),
			len(expected),
		)
	}

	return nil
}

func isTaskType(value string) bool {
	switch value {
	case "bugfix", "feature", "refactor", "tests", "code_review", "architecture", "devops", "research", "documentation", "other":
		return true
	default:
		return false
	}
}

func taskTypeCacheKey(task sessions.Interaction) string {
	hash := sha256.New()

	hash.Write([]byte(taskTypeCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(task.TurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(task.Prompt))

	return "task-type:" + hex.EncodeToString(hash.Sum(nil))
}

func taskTypeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"turn_id": map[string]any{
							"type": "string",
						},
						"type": map[string]any{
							"type": "string",
							"enum": []string{
								"bugfix",
								"feature",
								"refactor",
								"tests",
								"code_review",
								"architecture",
								"devops",
								"research",
								"documentation",
								"other",
							},
						},
						"confidence": map[string]any{
							"type": "number",
						},
					},
					"required": []string{
						"turn_id",
						"type",
						"confidence",
					},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"results"},
		"additionalProperties": false,
	}
}
