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

const promptQualityCacheVersion = "prompt-quality-v1"

// PromptQualityResult identifies the primary prompt defect for a steering case.
type PromptQualityResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Issue          string  `json:"issue"`
	Confidence     float64 `json:"confidence"`
}

type PromptQualityAnalysis struct {
	Results   []PromptQualityResult
	CacheHits int
	Evaluated int
}

type promptQualityResponse struct {
	Results []PromptQualityResult `json:"results"`
}

type PromptQualityAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewPromptQualityAnalyzer() (PromptQualityAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return PromptQualityAnalyzer{}, err
	}

	return PromptQualityAnalyzer{runner: judge.New(), cache: store, batchSize: 10}, nil
}

func (a PromptQualityAnalyzer) Analyze(
	followups []sessions.Followup,
	preventionResults []PreventionResult,
) (PromptQualityAnalysis, error) {
	cases := preventionCases(followups, preventionResults, "task_prompt")
	analysis := PromptQualityAnalysis{Results: make([]PromptQualityResult, 0, len(cases))}
	resultsByTurn := make(map[string]PromptQualityResult, len(cases))
	pending := make([]sessions.Followup, 0, len(cases))

	for _, item := range cases {
		var cached PromptQualityResult
		if a.cache != nil && a.cache.Get(promptQualityCacheKey(item), &cached) && cached.PreviousTurnID == item.PreviousTurnID && isPromptQualityIssue(cached.Issue) {
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
			return PromptQualityAnalysis{}, fmt.Errorf("analyze prompt quality batch %d-%d: %w", start, end, err)
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
				if err := a.cache.Set(promptQualityCacheKey(item), result); err != nil {
					return PromptQualityAnalysis{}, err
				}
			}
			if err := a.cache.Save(); err != nil {
				return PromptQualityAnalysis{}, err
			}
		}
	}

	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return PromptQualityAnalysis{}, fmt.Errorf("missing prompt quality result for turn %q", item.PreviousTurnID)
		}
		analysis.Results = append(analysis.Results, result)
	}
	return analysis, nil
}

func (a PromptQualityAnalyzer) analyzeBatch(cases []sessions.Followup) ([]PromptQualityResult, error) {
	records := make([]followupJudgeRecord, 0, len(cases))
	for _, item := range cases {
		records = append(records, followupJudgeRecord{PreviousTurnID: item.PreviousTurnID, PreviousAnswer: item.PreviousAnswer, Followup: item.Prompt})
	}
	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal prompt quality cases: %w", err)
	}
	prompt := `Classify the primary missing quality in the user's task prompt for each steering case.

Issues:
missing_constraints = important requirements or limitations were not stated
missing_context = necessary project, domain, or input context was not stated
ambiguous_request = the request allowed materially different interpretations
missing_acceptance = the desired completion or success criteria were not stated
missing_scope = boundaries of what to change or not change were not stated
wrong_assumption = the prompt contained or implied a materially wrong assumption
other = none of the above

Choose exactly one primary issue per record. Return every record exactly once. Do not use tools.

Records:
` + string(data)
	var response promptQualityResponse
	if err := a.runner.Run(prompt, promptQualitySchema(), &response); err != nil {
		return nil, err
	}
	if err := validatePromptQualityResults(cases, response.Results); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func validatePromptQualityResults(cases []sessions.Followup, results []PromptQualityResult) error {
	expected := expectedFollowupIDs(cases)
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := expected[result.PreviousTurnID]; !ok {
			return fmt.Errorf("judge returned unexpected turn id %q", result.PreviousTurnID)
		}
		if _, duplicate := seen[result.PreviousTurnID]; duplicate {
			return fmt.Errorf("judge returned duplicate turn id %q", result.PreviousTurnID)
		}
		if !isPromptQualityIssue(result.Issue) {
			return fmt.Errorf("judge returned unsupported prompt quality issue %q", result.Issue)
		}
		seen[result.PreviousTurnID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("judge returned %d results, expected %d", len(seen), len(expected))
	}
	return nil
}

func promptQualityCacheKey(item sessions.Followup) string {
	hash := sha256.New()
	hash.Write([]byte(promptQualityCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))
	return "prompt-quality:" + hex.EncodeToString(hash.Sum(nil))
}

func isPromptQualityIssue(value string) bool {
	switch value {
	case "missing_constraints", "missing_context", "ambiguous_request", "missing_acceptance", "missing_scope", "wrong_assumption", "other":
		return true
	default:
		return false
	}
}

func promptQualitySchema() map[string]any {
	return followupClassifierSchema("issue", []string{"missing_constraints", "missing_context", "ambiguous_request", "missing_acceptance", "missing_scope", "wrong_assumption", "other"})
}
