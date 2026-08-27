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

const validationCacheVersion = "validation-v1"

type ValidationResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	ValidationType string  `json:"validation_type"`
	Confidence     float64 `json:"confidence"`
}

type ValidationAnalysis struct {
	Results   []ValidationResult
	CacheHits int
	Evaluated int
}

type validationResponse struct {
	Results []ValidationResult `json:"results"`
}

type ValidationAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewValidationAnalyzer() (ValidationAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return ValidationAnalyzer{}, err
	}

	return ValidationAnalyzer{
		runner:    judge.New(),
		cache:     store,
		batchSize: 10,
	}, nil
}

func (a ValidationAnalyzer) Analyze(
	followups []sessions.Followup,
	preventionResults []PreventionResult,
) (ValidationAnalysis, error) {
	validationIDs := make(map[string]struct{})

	for _, result := range preventionResults {
		if containsString(result.Applicable, "validation") {
			validationIDs[result.PreviousTurnID] = struct{}{}
		}
	}

	cases := make([]sessions.Followup, 0, len(validationIDs))

	for _, followup := range followups {
		if _, ok := validationIDs[followup.PreviousTurnID]; ok {
			cases = append(cases, followup)
		}
	}

	analysis := ValidationAnalysis{
		Results: make([]ValidationResult, 0, len(cases)),
	}

	resultsByTurn := make(
		map[string]ValidationResult,
		len(cases),
	)

	pending := make([]sessions.Followup, 0, len(cases))

	for _, item := range cases {
		var cached ValidationResult

		if a.cache != nil &&
			a.cache.Get(validationCacheKey(item), &cached) &&
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

		results, err := analyzeBatchWithSplit(batch, a.analyzeBatch)
		if err != nil {
			return ValidationAnalysis{}, fmt.Errorf(
				"analyze validation batch %d-%d: %w",
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
					validationCacheKey(item),
					result,
				); err != nil {
					return ValidationAnalysis{}, err
				}
			}

			if err := a.cache.Save(); err != nil {
				return ValidationAnalysis{}, err
			}
		}
	}

	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return ValidationAnalysis{}, fmt.Errorf(
				"missing validation result for turn %q",
				item.PreviousTurnID,
			)
		}

		analysis.Results = append(analysis.Results, result)
	}

	return analysis, nil
}

func (a ValidationAnalyzer) analyzeBatch(
	cases []sessions.Followup,
) ([]ValidationResult, error) {
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
		return nil, fmt.Errorf("marshal validation cases: %w", err)
	}

	prompt := `Determine the primary validation mechanism that most likely
would have caught this failure before Codex declared completion.

Validation types:
tests = unit, integration, end-to-end, regression, or behavioral tests
static_analysis = lint, type checking, compiler, PHPStan, or similar static checks
runtime_smoke_check = actually run the application, command, service, or feature and verify behavior
deployment_check = verify CI/CD, Kubernetes, deployment, infrastructure, monitoring, or post-deploy state
output_validation = inspect generated file, Markdown, PDF, artifact, format, or content
diff_review = review the produced code or diff for unintended changes, regressions, or scope creep
data_validation = verify SQL, API responses, calculations, metrics, or actual data
other = none of the above

Choose exactly one primary validation type per record.
Evaluate only the supplied conversation pairs.
Return every record exactly once.
Do not use tools.

Records:
` + string(data)

	var response validationResponse

	if err := a.runner.Run(
		prompt,
		validationSchema(),
		&response,
	); err != nil {
		return nil, err
	}

	if err := validateValidationResults(
		cases,
		response.Results,
	); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func validateValidationResults(
	cases []sessions.Followup,
	results []ValidationResult,
) error {
	expected := make(map[string]struct{}, len(cases))

	for _, item := range cases {
		expected[item.PreviousTurnID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(results))

	allowed := map[string]struct{}{
		"tests":               {},
		"static_analysis":     {},
		"runtime_smoke_check": {},
		"deployment_check":    {},
		"output_validation":   {},
		"diff_review":         {},
		"data_validation":     {},
		"other":               {},
	}

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

		if _, ok := allowed[result.ValidationType]; !ok {
			return fmt.Errorf(
				"judge returned unsupported validation type %q",
				result.ValidationType,
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

func validationCacheKey(item sessions.Followup) string {
	hash := sha256.New()

	hash.Write([]byte(validationCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))

	return "validation:" + hex.EncodeToString(hash.Sum(nil))
}

func validationSchema() map[string]any {
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
						"validation_type": map[string]any{
							"type": "string",
							"enum": []string{
								"tests",
								"static_analysis",
								"runtime_smoke_check",
								"deployment_check",
								"output_validation",
								"diff_review",
								"data_validation",
								"other",
							},
						},
						"confidence": map[string]any{
							"type": "number",
						},
					},
					"required": []string{
						"previous_turn_id",
						"validation_type",
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
