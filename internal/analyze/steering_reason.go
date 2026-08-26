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

const steeringReasonCacheVersion = "steering-reason-v1"

type SteeringReasonResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Reason         string  `json:"reason"`
	Confidence     float64 `json:"confidence"`
}

type SteeringReasonAnalysis struct {
	Results   []SteeringReasonResult
	CacheHits int
	Evaluated int
}

type steeringReasonResponse struct {
	Results []SteeringReasonResult `json:"results"`
}

type SteeringReasonAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewSteeringReasonAnalyzer() (SteeringReasonAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return SteeringReasonAnalyzer{}, err
	}

	return SteeringReasonAnalyzer{
		runner:    judge.New(),
		cache:     store,
		batchSize: 10,
	}, nil
}

func (a SteeringReasonAnalyzer) Analyze(
	followups []sessions.Followup,
	steeringResults []SteeringResult,
) (SteeringReasonAnalysis, error) {
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

	analysis := SteeringReasonAnalysis{
		Results: make([]SteeringReasonResult, 0, len(cases)),
	}

	resultsByTurn := make(
		map[string]SteeringReasonResult,
		len(cases),
	)

	pending := make([]sessions.Followup, 0, len(cases))

	for _, item := range cases {
		var cached SteeringReasonResult

		if a.cache != nil &&
			a.cache.Get(steeringReasonCacheKey(item), &cached) &&
			cached.PreviousTurnID == item.PreviousTurnID {

			resultsByTurn[item.PreviousTurnID] = cached
			analysis.CacheHits++
			continue
		}

		pending = append(pending, item)
	}

	for start := 0; start < len(pending); start += a.batchSize {
		end := start + a.batchSize
		if end > len(pending) {
			end = len(pending)
		}

		batch := pending[start:end]

		results, err := a.analyzeBatch(batch)
		if err != nil {
			return SteeringReasonAnalysis{}, fmt.Errorf(
				"analyze steering reason batch %d-%d: %w",
				start,
				end,
				err,
			)
		}

		analysis.Evaluated += len(batch)

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
					steeringReasonCacheKey(item),
					result,
				); err != nil {
					return SteeringReasonAnalysis{}, err
				}
			}

			if err := a.cache.Save(); err != nil {
				return SteeringReasonAnalysis{}, err
			}
		}
	}

	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return SteeringReasonAnalysis{}, fmt.Errorf(
				"missing steering reason for turn %q",
				item.PreviousTurnID,
			)
		}

		analysis.Results = append(analysis.Results, result)
	}

	return analysis, nil
}

func (a SteeringReasonAnalyzer) analyzeBatch(
	cases []sessions.Followup,
) ([]SteeringReasonResult, error) {
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

	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal steering cases: %w", err)
	}

	prompt := `Determine the primary reason the user had to correct or redirect Codex.

Reasons:
misunderstood_request = Codex misunderstood what the user wanted
overengineering = Codex added unnecessary complexity or extra work
implementation_error = generated implementation was technically incorrect
ignored_constraints = Codex ignored an explicit requirement or limitation
insufficient_validation = Codex failed to verify, test, or check its work sufficiently
architecture_mismatch = solution did not fit project architecture or conventions
wrong_output_format = result was delivered in the wrong requested format
other = none of the above

Choose exactly one primary reason per record.
Evaluate only the supplied conversation pairs.
Return every record exactly once.
Do not use tools.

Records:
` + string(data)

	var response steeringReasonResponse

	if err := a.runner.Run(
		prompt,
		steeringReasonSchema(),
		&response,
	); err != nil {
		return nil, err
	}

	if err := validateSteeringReasonResults(
		cases,
		response.Results,
	); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func validateSteeringReasonResults(
	cases []sessions.Followup,
	results []SteeringReasonResult,
) error {
	expected := make(map[string]struct{}, len(cases))

	for _, item := range cases {
		expected[item.PreviousTurnID] = struct{}{}
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

func steeringReasonCacheKey(item sessions.Followup) string {
	hash := sha256.New()

	hash.Write([]byte(steeringReasonCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))

	return "steering-reason:" + hex.EncodeToString(hash.Sum(nil))
}

func steeringReasonSchema() map[string]any {
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
						"reason": map[string]any{
							"type": "string",
							"enum": []string{
								"misunderstood_request",
								"overengineering",
								"implementation_error",
								"ignored_constraints",
								"insufficient_validation",
								"architecture_mismatch",
								"wrong_output_format",
								"other",
							},
						},
						"confidence": map[string]any{
							"type": "number",
						},
					},
					"required": []string{
						"previous_turn_id",
						"reason",
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
