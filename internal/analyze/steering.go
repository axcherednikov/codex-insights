package analyze

import (
	"encoding/json"
	"fmt"

	"codex-insights/internal/judge"
	"codex-insights/internal/sessions"
)

type SteeringResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Label          string  `json:"label"`
	Confidence     float64 `json:"confidence"`
}

type steeringResponse struct {
	Results []SteeringResult `json:"results"`
}

type SteeringAnalyzer struct {
	runner    judge.Runner
	batchSize int
}

func NewSteeringAnalyzer() SteeringAnalyzer {
	return SteeringAnalyzer{
		runner:    judge.New(),
		batchSize: 25,
	}
}

func (a SteeringAnalyzer) Analyze(
	followups []sessions.Followup,
) ([]SteeringResult, error) {
	var results []SteeringResult

	for start := 0; start < len(followups); start += a.batchSize {
		end := start + a.batchSize
		if end > len(followups) {
			end = len(followups)
		}

		batch := followups[start:end]

		batchResults, err := a.analyzeBatch(batch)
		if err != nil {
			return nil, fmt.Errorf(
				"analyze follow-up batch %d-%d: %w",
				start,
				end,
				err,
			)
		}

		results = append(results, batchResults...)
	}

	return results, nil
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

	if err := validateSteeringResults(followups, response.Results); err != nil {
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
