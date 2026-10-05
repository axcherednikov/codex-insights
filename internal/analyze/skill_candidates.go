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

const skillCandidatesCacheVersion = "skill-candidates-v1"

const skillCandidatesBatchSize = 10

type SkillCandidateResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Category       string  `json:"category"`
	Confidence     float64 `json:"confidence"`
}

type SkillCandidatesAnalysis struct {
	Results   []SkillCandidateResult
	CacheHits int
	Evaluated int
}

type skillCandidatesResponse struct {
	Results []SkillCandidateResult `json:"results"`
}

type SkillCandidatesAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewSkillCandidatesAnalyzer() (SkillCandidatesAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return SkillCandidatesAnalyzer{}, fmt.Errorf("open analysis cache: %w", err)
	}

	return SkillCandidatesAnalyzer{runner: judge.New(), cache: store, batchSize: skillCandidatesBatchSize}, nil
}

func (a SkillCandidatesAnalyzer) Analyze(
	followups []sessions.Followup,
	preventionResults []PreventionResult,
) (SkillCandidatesAnalysis, error) {
	cases := preventionCases(followups, preventionResults, "skill")
	analysis := SkillCandidatesAnalysis{Results: make([]SkillCandidateResult, 0, len(cases))}
	resultsByTurn, pending, cacheHits := a.selectPending(cases)
	analysis.CacheHits = cacheHits

	var err error
	analysis.Evaluated, err = a.evaluatePending(pending, resultsByTurn)
	if err != nil {
		return SkillCandidatesAnalysis{}, fmt.Errorf("run analysis: %w", err)
	}

	analysis.Results, err = assembleSkillCandidatesAnalysis(cases, resultsByTurn)
	if err != nil {
		return SkillCandidatesAnalysis{}, err
	}

	return analysis, nil
}

func (a SkillCandidatesAnalyzer) selectPending(
	cases []sessions.Followup,
) (map[string]SkillCandidateResult, []sessions.Followup, int) {
	resultsByTurn := make(map[string]SkillCandidateResult, len(cases))
	pending := make([]sessions.Followup, 0, len(cases))
	cacheHits := 0
	for _, item := range cases {
		var cached SkillCandidateResult
		if a.cache != nil && a.cache.Get(skillCandidatesCacheKey(item), &cached) && cached.PreviousTurnID == item.PreviousTurnID && isSkillCandidate(cached.Category) {
			resultsByTurn[item.PreviousTurnID] = cached
			cacheHits++

			continue
		}
		pending = append(pending, item)
	}

	return resultsByTurn, pending, cacheHits
}

func assembleSkillCandidatesAnalysis(
	cases []sessions.Followup,
	resultsByTurn map[string]SkillCandidateResult,
) ([]SkillCandidateResult, error) {
	results := make([]SkillCandidateResult, 0, len(cases))
	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return nil, fmt.Errorf("%w %q", errMissingSkillCandidateResult, item.PreviousTurnID)
		}
		results = append(results, result)
	}

	return results, nil
}

func (a SkillCandidatesAnalyzer) evaluatePending(
	pending []sessions.Followup,
	resultsByTurn map[string]SkillCandidateResult,
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
			return 0, fmt.Errorf("analyze Skill candidates batch %d-%d: %w", start, end, err)
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
				if err := a.cache.Set(skillCandidatesCacheKey(item), result); err != nil {
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

func (a SkillCandidatesAnalyzer) analyzeBatch(cases []sessions.Followup) ([]SkillCandidateResult, error) {
	records := make([]followupJudgeRecord, 0, len(cases))
	for _, item := range cases {
		records = append(records, followupJudgeRecord{PreviousTurnID: item.PreviousTurnID, PreviousAnswer: item.PreviousAnswer, Followup: item.Prompt})
	}
	// The concrete record type contains only strings, so JSON marshaling cannot fail.
	data, _ := json.Marshal(records)
	prompt := `Classify each skill-prevention case into one reusable workflow category.

Categories:
implementation_prompt_file = a reusable workflow for turning a request into a precise implementation prompt file for another implementation worker
bounded_architecture_review = a reusable workflow for a focused architecture review with explicit boundaries and existing-code inspection
verification_workflow = a reusable workflow for systematic testing, validation, or artifact verification before completion
other = noise, an isolated domain-specific need, or no reusable workflow should be generalized

Choose exactly one category per record. Generalize only workflows useful across multiple projects; use other for highly specific cases. Return every record exactly once. Do not use tools.

Records:
` + string(data)
	var response skillCandidatesResponse
	if err := a.runner.Run(prompt, skillCandidatesSchema(), &response); err != nil {
		return nil, fmt.Errorf("run Judge classification: %w", err)
	}
	if err := validateSkillCandidatesResults(cases, response.Results); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func validateSkillCandidatesResults(cases []sessions.Followup, results []SkillCandidateResult) error {
	expected := expectedFollowupIDs(cases)
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := expected[result.PreviousTurnID]; !ok {
			return fmt.Errorf("%w %q", errUnexpectedResultID, result.PreviousTurnID)
		}
		if _, duplicate := seen[result.PreviousTurnID]; duplicate {
			return fmt.Errorf("%w %q", errDuplicateResultID, result.PreviousTurnID)
		}
		if !isSkillCandidate(result.Category) {
			return fmt.Errorf("%w %q", errUnsupportedSkillCandidate, result.Category)
		}
		seen[result.PreviousTurnID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w %d results, expected %d", errJudgeResultCount, len(seen), len(expected))
	}

	return nil
}

func skillCandidatesCacheKey(item sessions.Followup) string {
	hash := sha256.New()
	hash.Write([]byte(skillCandidatesCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))

	return "skill-candidates:" + hex.EncodeToString(hash.Sum(nil))
}

func isSkillCandidate(value string) bool {
	switch value {
	case "implementation_prompt_file", "bounded_architecture_review", "verification_workflow", "other":
		return true
	default:
		return false
	}
}

func skillCandidatesSchema() map[string]any {
	return followupClassifierSchema("category", []string{"implementation_prompt_file", "bounded_architecture_review", "verification_workflow", "other"})
}
