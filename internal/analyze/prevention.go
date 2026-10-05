package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	analysiscache "github.com/axcherednikov/codex-insights/internal/cache"
	"github.com/axcherednikov/codex-insights/internal/judge"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

const preventionCacheVersion = "prevention-v1"

const preventionBatchSize = 10

type PreventionResult struct {
	PreviousTurnID string   `json:"previous_turn_id"`
	Applicable     []string `json:"applicable"`
	NotPreventable bool     `json:"not_preventable"`
	Confidence     float64  `json:"confidence"`
}

type PreventionAnalysis struct {
	Results   []PreventionResult
	CacheHits int
	Evaluated int
}

type preventionResponse struct {
	Results []PreventionResult `json:"results"`
}

type PreventionAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewPreventionAnalyzer() (PreventionAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return PreventionAnalyzer{}, fmt.Errorf("open analysis cache: %w", err)
	}

	return PreventionAnalyzer{
		runner:    judge.New(),
		cache:     store,
		batchSize: preventionBatchSize,
	}, nil
}

func (a PreventionAnalyzer) Analyze(
	followups []sessions.Followup,
	steeringResults []SteeringResult,
) (PreventionAnalysis, error) {
	steeringIDs := make(map[string]struct{})

	for _, result := range steeringResults {
		if result.Label == "steering" {
			steeringIDs[result.PreviousTurnID] = struct{}{}
		}
	}

	cases := make([]sessions.Followup, 0, len(steeringIDs))

	for _, followup := range followups {
		if _, ok := steeringIDs[followup.PreviousTurnID]; ok {
			cases = append(cases, followup)
		}
	}

	analysis := PreventionAnalysis{
		Results: make([]PreventionResult, 0, len(cases)),
	}

	resultsByTurn, pending, cacheHits := a.selectPending(cases)
	analysis.CacheHits = cacheHits

	var err error
	analysis.Evaluated, err = a.evaluatePending(pending, resultsByTurn)
	if err != nil {
		return PreventionAnalysis{}, fmt.Errorf("run analysis: %w", err)
	}

	analysis.Results, err = assemblePreventionAnalysis(cases, resultsByTurn)
	if err != nil {
		return PreventionAnalysis{}, err
	}

	return analysis, nil
}

func (a PreventionAnalyzer) selectPending(
	cases []sessions.Followup,
) (map[string]PreventionResult, []sessions.Followup, int) {
	resultsByTurn := make(map[string]PreventionResult, len(cases))
	pending := make([]sessions.Followup, 0, len(cases))
	cacheHits := 0
	for _, item := range cases {
		var cached PreventionResult
		if a.cache != nil && a.cache.Get(preventionCacheKey(item), &cached) && cached.PreviousTurnID == item.PreviousTurnID {
			resultsByTurn[item.PreviousTurnID] = cached
			cacheHits++

			continue
		}
		pending = append(pending, item)
	}

	return resultsByTurn, pending, cacheHits
}

func assemblePreventionAnalysis(
	cases []sessions.Followup,
	resultsByTurn map[string]PreventionResult,
) ([]PreventionResult, error) {
	results := make([]PreventionResult, 0, len(cases))
	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return nil, fmt.Errorf("%w %q", errMissingPreventionResult, item.PreviousTurnID)
		}
		results = append(results, result)
	}

	return results, nil
}

func (a PreventionAnalyzer) evaluatePending(
	pending []sessions.Followup,
	resultsByTurn map[string]PreventionResult,
) (int, error) {
	var evaluated int

	for start := 0; start < len(pending); start += a.batchSize {
		end := start + a.batchSize
		if end > len(pending) {
			end = len(pending)
		}

		batch := pending[start:end]

		results, err := analyzeBatchWithSplit(batch, a.analyzeBatch)
		if err != nil {
			return 0, fmt.Errorf(
				"analyze prevention batch %d-%d: %w",
				start,
				end,
				err,
			)
		}

		evaluated += len(batch)

		for _, result := range results {
			resultsByTurn[result.PreviousTurnID] = result
		}

		if a.cache != nil {
			for _, item := range batch {
				result, ok := resultsByTurn[item.PreviousTurnID]
				if !ok {
					continue
				}

				if err := a.cache.Set(
					preventionCacheKey(item),
					result,
				); err != nil {
					return 0, fmt.Errorf("persist analysis cache: %w", err)
				}
			}

			if err := a.cache.Save(); err != nil {
				return 0, fmt.Errorf("persist analysis cache: %w", err)
			}
		}
	}

	return evaluated, nil
}

func (a PreventionAnalyzer) analyzeBatch(
	cases []sessions.Followup,
) ([]PreventionResult, error) {
	type record struct {
		PreviousTurnID string `json:"previous_turn_id"`
		PreviousAnswer string `json:"previous_answer"`
		Followup       string `json:"followup"`
	}

	records := make([]record, 0, len(cases))

	for _, item := range cases {
		records = append(records, record{
			PreviousTurnID: item.PreviousTurnID,
			PreviousAnswer: item.PreviousAnswer,
			Followup:       item.Prompt,
		})
	}

	// The concrete record type contains only strings, so JSON marshaling cannot fail.
	data, _ := json.Marshal(records)

	prompt := `Identify prevention mechanisms for each steering case.

Be strict.

Mark a mechanism ONLY if it would have a high probability of preventing
or automatically catching THIS specific failure.

Prefer 0-2 mechanisms per case. More than 2 should be exceptional.

Mechanisms:
agents_md = a stable project-wide rule would likely prevent the mistake
skill = a reusable workflow for this task type would likely prevent the mistake
task_prompt = missing or ambiguous information in THIS request materially caused the mistake
validation = an automated test, check, or review would likely catch the mistake before completion

Multiple mechanisms may apply.

If none clearly qualify:
applicable = []
not_preventable = true

Otherwise:
not_preventable = false

Evaluate only the supplied conversation pairs.
Return every record exactly once.
Do not use tools.

Records:
` + string(data)

	var response preventionResponse

	if err := a.runner.Run(
		prompt,
		preventionSchema(),
		&response,
	); err != nil {
		return nil, fmt.Errorf("run Judge classification: %w", err)
	}

	results, err := normalizeAndValidatePreventionResults(
		cases,
		response.Results,
	)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func normalizeAndValidatePreventionResults(
	cases []sessions.Followup,
	results []PreventionResult,
) ([]PreventionResult, error) {
	expected := make(map[string]struct{}, len(cases))

	for _, item := range cases {
		expected[item.PreviousTurnID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(results))

	allowed := map[string]struct{}{
		"agents_md":   {},
		"skill":       {},
		"task_prompt": {},
		"validation":  {},
	}

	normalized := make([]PreventionResult, 0, len(results))

	for _, result := range results {
		if _, ok := expected[result.PreviousTurnID]; !ok {
			return nil, fmt.Errorf("%w %q", errUnexpectedResultID, result.PreviousTurnID)
		}

		if _, duplicate := seen[result.PreviousTurnID]; duplicate {
			return nil, fmt.Errorf("%w %q", errDuplicateResultID, result.PreviousTurnID)
		}

		seen[result.PreviousTurnID] = struct{}{}

		unique := make([]string, 0, len(result.Applicable))
		used := make(map[string]struct{})

		for _, mechanism := range result.Applicable {
			if _, ok := allowed[mechanism]; !ok {
				return nil, fmt.Errorf("%w %q", errUnsupportedPrevention, mechanism)
			}

			if _, duplicate := used[mechanism]; duplicate {
				continue
			}

			used[mechanism] = struct{}{}
			unique = append(unique, mechanism)
		}

		result.Applicable = unique

		if result.NotPreventable && len(result.Applicable) > 0 {
			return nil, fmt.Errorf("%w %q is marked not preventable but has applicable mechanisms", errNotPreventableHasMechanism, result.PreviousTurnID)
		}

		if !result.NotPreventable && len(result.Applicable) == 0 {
			return nil, fmt.Errorf("%w %q has no prevention mechanism but is not marked not preventable", errPreventableWithoutMechanism, result.PreviousTurnID)
		}

		normalized = append(normalized, result)
	}

	if len(seen) != len(expected) {
		return nil, fmt.Errorf("%w %d results, expected %d", errJudgeResultCount, len(seen), len(expected))
	}

	return normalized, nil
}

func preventionCacheKey(item sessions.Followup) string {
	hash := sha256.New()

	hash.Write([]byte(preventionCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))

	return "prevention:" + hex.EncodeToString(hash.Sum(nil))
}

func preventionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"previous_turn_id": map[string]any{
							"type": "string",
						},
						"applicable": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
								"enum": []string{
									"agents_md",
									"skill",
									"task_prompt",
									"validation",
								},
							},
						},
						"not_preventable": map[string]any{
							"type": "boolean",
						},
						"confidence": map[string]any{
							"type": "number",
						},
					},
					"required": []string{
						"previous_turn_id",
						"applicable",
						"not_preventable",
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
