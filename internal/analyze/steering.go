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

const steeringCacheVersion = "steering-v1"

type SteeringResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Label          string  `json:"label"`
	Confidence     float64 `json:"confidence"`
}

type SteeringAnalysis struct {
	Results   []SteeringResult
	CacheHits int
	Evaluated int
}

type steeringResponse struct {
	Results []SteeringResult `json:"results"`
}

type SteeringAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewSteeringAnalyzer() (SteeringAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return SteeringAnalyzer{}, err
	}

	return SteeringAnalyzer{
		runner:    judge.New(),
		cache:     store,
		batchSize: 10,
	}, nil
}

func (a SteeringAnalyzer) Analyze(
	followups []sessions.Followup,
) (SteeringAnalysis, error) {
	analysis := SteeringAnalysis{
		Results: make([]SteeringResult, 0, len(followups)),
	}

	resultsByTurn := make(
		map[string]SteeringResult,
		len(followups),
	)

	pending := make([]sessions.Followup, 0, len(followups))

	for _, followup := range followups {
		var cached SteeringResult

		if a.cache != nil &&
			a.cache.Get(steeringCacheKey(followup), &cached) &&
			cached.PreviousTurnID == followup.PreviousTurnID {

			resultsByTurn[followup.PreviousTurnID] = cached
			analysis.CacheHits++
			continue
		}

		pending = append(pending, followup)
	}

	for start := 0; start < len(pending); start += a.batchSize {
		end := start + a.batchSize
		if end > len(pending) {
			end = len(pending)
		}

		batch := pending[start:end]

		batchResults, err := analyzeBatchWithSplit(batch, a.analyzeBatch)
		if err != nil {
			return SteeringAnalysis{}, fmt.Errorf(
				"analyze follow-up batch %d-%d: %w",
				start,
				end,
				err,
			)
		}

		analysis.Evaluated += len(batch)

		for _, result := range batchResults {
			resultsByTurn[result.PreviousTurnID] = result
		}

		if a.cache != nil {
			for _, followup := range batch {
				result, ok := resultsByTurn[followup.PreviousTurnID]
				if !ok {
					continue
				}

				if err := a.cache.Set(
					steeringCacheKey(followup),
					result,
				); err != nil {
					return SteeringAnalysis{}, err
				}
			}

			if err := a.cache.Save(); err != nil {
				return SteeringAnalysis{}, err
			}
		}
	}

	for _, followup := range followups {
		result, ok := resultsByTurn[followup.PreviousTurnID]
		if !ok {
			return SteeringAnalysis{}, fmt.Errorf(
				"missing steering result for turn %q",
				followup.PreviousTurnID,
			)
		}

		analysis.Results = append(analysis.Results, result)
	}

	return analysis, nil
}

func (a SteeringAnalyzer) analyzeBatch(
	followups []sessions.Followup,
) ([]SteeringResult, error) {
	type record struct {
		PreviousTurnID string `json:"previous_turn_id"`
		PreviousAnswer string `json:"previous_answer"`
		Followup       string `json:"followup"`
	}

	records := make([]record, 0, len(followups))

	for _, followup := range followups {
		records = append(records, record{
			PreviousTurnID: followup.PreviousTurnID,
			PreviousAnswer: followup.PreviousAnswer,
			Followup:       followup.Prompt,
		})
	}

	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal follow-ups: %w", err)
	}

	prompt := `Classify every conversation follow-up.

Labels:
steering = user corrects or redirects Codex because previous work or understanding was wrong
continuation = normal next step or additional request
user_correction = user changes or corrects their own requirement, not a Codex mistake
question = user mainly asks a question or clarification

Evaluate only the supplied conversation pairs.
Return every record exactly once.
Do not use tools.

Records:
` + string(data)

	var response steeringResponse

	if err := a.runner.Run(
		prompt,
		steeringSchema(),
		&response,
	); err != nil {
		return nil, err
	}

	if err := validateSteeringResults(
		followups,
		response.Results,
	); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func validateSteeringResults(
	followups []sessions.Followup,
	results []SteeringResult,
) error {
	expected := make(map[string]struct{}, len(followups))

	for _, followup := range followups {
		expected[followup.PreviousTurnID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(results))

	for _, result := range results {
		if _, ok := expected[result.PreviousTurnID]; !ok {
			return fmt.Errorf(
				"judge returned unexpected turn id %q",
				result.PreviousTurnID,
			)
		}

		if _, duplicate := seen[result.PreviousTurnID]; duplicate {
			return fmt.Errorf(
				"judge returned duplicate turn id %q",
				result.PreviousTurnID,
			)
		}

		if !isSteeringLabel(result.Label) {
			return fmt.Errorf(
				"judge returned unsupported steering label %q",
				result.Label,
			)
		}

		seen[result.PreviousTurnID] = struct{}{}
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

func isSteeringLabel(value string) bool {
	switch value {
	case "steering", "continuation", "user_correction", "question":
		return true
	default:
		return false
	}
}

func steeringCacheKey(followup sessions.Followup) string {
	hash := sha256.New()

	hash.Write([]byte(steeringCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(followup.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(followup.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(followup.Prompt))

	return "steering:" + hex.EncodeToString(hash.Sum(nil))
}

func steeringSchema() map[string]any {
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
						"label": map[string]any{
							"type": "string",
							"enum": []string{
								"steering",
								"continuation",
								"user_correction",
								"question",
							},
						},
						"confidence": map[string]any{
							"type": "number",
						},
					},
					"required": []string{
						"previous_turn_id",
						"label",
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
