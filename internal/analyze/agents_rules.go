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

const agentsRulesCacheVersion = "agents-rules-v1"

type AgentsRuleResult struct {
	PreviousTurnID string  `json:"previous_turn_id"`
	Rule           string  `json:"rule"`
	Confidence     float64 `json:"confidence"`
}

type AgentsRulesAnalysis struct {
	Results   []AgentsRuleResult
	CacheHits int
	Evaluated int
}

type agentsRulesResponse struct {
	Results []AgentsRuleResult `json:"results"`
}

type AgentsRulesAnalyzer struct {
	runner    judge.Runner
	cache     *analysiscache.Store
	batchSize int
}

func NewAgentsRulesAnalyzer() (AgentsRulesAnalyzer, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return AgentsRulesAnalyzer{}, err
	}
	return AgentsRulesAnalyzer{runner: judge.New(), cache: store, batchSize: 10}, nil
}

func (a AgentsRulesAnalyzer) Analyze(
	followups []sessions.Followup,
	preventionResults []PreventionResult,
) (AgentsRulesAnalysis, error) {
	cases := preventionCases(followups, preventionResults, "agents_md")
	analysis := AgentsRulesAnalysis{Results: make([]AgentsRuleResult, 0, len(cases))}
	resultsByTurn := make(map[string]AgentsRuleResult, len(cases))
	pending := make([]sessions.Followup, 0, len(cases))
	for _, item := range cases {
		var cached AgentsRuleResult
		if a.cache != nil && a.cache.Get(agentsRulesCacheKey(item), &cached) && cached.PreviousTurnID == item.PreviousTurnID && isAgentsRule(cached.Rule) {
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
			return AgentsRulesAnalysis{}, fmt.Errorf("analyze AGENTS.md rules batch %d-%d: %w", start, end, err)
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
				if err := a.cache.Set(agentsRulesCacheKey(item), result); err != nil {
					return AgentsRulesAnalysis{}, err
				}
			}
			if err := a.cache.Save(); err != nil {
				return AgentsRulesAnalysis{}, err
			}
		}
	}
	for _, item := range cases {
		result, ok := resultsByTurn[item.PreviousTurnID]
		if !ok {
			return AgentsRulesAnalysis{}, fmt.Errorf("missing AGENTS.md rule result for turn %q", item.PreviousTurnID)
		}
		analysis.Results = append(analysis.Results, result)
	}
	return analysis, nil
}

func (a AgentsRulesAnalyzer) analyzeBatch(cases []sessions.Followup) ([]AgentsRuleResult, error) {
	records := make([]followupJudgeRecord, 0, len(cases))
	for _, item := range cases {
		records = append(records, followupJudgeRecord{PreviousTurnID: item.PreviousTurnID, PreviousAnswer: item.PreviousAnswer, Followup: item.Prompt})
	}
	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal AGENTS.md rule cases: %w", err)
	}
	prompt := `Classify the primary recurring project-level rule that would have prevented the failure in each steering case.

Rules:
avoid_unnecessary_complexity = prefer the simplest sufficient implementation
preserve_project_architecture = fit existing structure, abstractions, and conventions
follow_explicit_constraints = obey stated requirements, limits, and prohibitions
avoid_scope_creep = change only the requested scope
inspect_existing_code_first = inspect relevant files and instructions before acting
validate_before_completion = run appropriate checks and verify the result before declaring completion
follow_requested_output_format = deliver the requested format or artifact
other = no stable project-level rule is a useful primary explanation

Choose exactly one primary rule per record. Use only rules that could be written as a recurring project instruction. Return every record exactly once. Do not use tools.

Records:
` + string(data)
	var response agentsRulesResponse
	if err := a.runner.Run(prompt, agentsRulesSchema(), &response); err != nil {
		return nil, err
	}
	if err := validateAgentsRulesResults(cases, response.Results); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func validateAgentsRulesResults(cases []sessions.Followup, results []AgentsRuleResult) error {
	expected := expectedFollowupIDs(cases)
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := expected[result.PreviousTurnID]; !ok {
			return fmt.Errorf("judge returned unexpected turn id %q", result.PreviousTurnID)
		}
		if _, duplicate := seen[result.PreviousTurnID]; duplicate {
			return fmt.Errorf("judge returned duplicate turn id %q", result.PreviousTurnID)
		}
		if !isAgentsRule(result.Rule) {
			return fmt.Errorf("judge returned unsupported AGENTS.md rule %q", result.Rule)
		}
		seen[result.PreviousTurnID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("judge returned %d results, expected %d", len(seen), len(expected))
	}
	return nil
}

func agentsRulesCacheKey(item sessions.Followup) string {
	hash := sha256.New()
	hash.Write([]byte(agentsRulesCacheVersion))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousTurnID))
	hash.Write([]byte{0})
	hash.Write([]byte(item.PreviousAnswer))
	hash.Write([]byte{0})
	hash.Write([]byte(item.Prompt))
	return "agents-rules:" + hex.EncodeToString(hash.Sum(nil))
}

func isAgentsRule(value string) bool {
	switch value {
	case "avoid_unnecessary_complexity", "preserve_project_architecture", "follow_explicit_constraints", "avoid_scope_creep", "inspect_existing_code_first", "validate_before_completion", "follow_requested_output_format", "other":
		return true
	default:
		return false
	}
}

func agentsRulesSchema() map[string]any {
	return followupClassifierSchema("rule", []string{"avoid_unnecessary_complexity", "preserve_project_architecture", "follow_explicit_constraints", "avoid_scope_creep", "inspect_existing_code_first", "validate_before_completion", "follow_requested_output_format", "other"})
}
