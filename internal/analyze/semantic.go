package analyze

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	analysiscache "github.com/axcherednikov/codex-insights/internal/cache"
	"github.com/axcherednikov/codex-insights/internal/judge"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

// These versions deliberately have independent lifecycles. Changing the
// methodology, prompt, or wire schema invalidates semantic cache entries
// without requiring a disk-format migration.
const (
	SemanticMethodologyVersion = "semantic-v1"
	SemanticPromptVersion      = "semantic-judge-v1"
	SemanticSchemaVersion      = "semantic-schema-v2"
	SemanticDefaultWorkers     = 6
	SemanticDefaultBatchSize   = 20
)

// SemanticFollowup is the language-independent input for a task's next turn.
// It is kept separate from sessions so semantic cache values never need to
// contain conversation text.
type SemanticFollowup struct {
	TurnID         string
	Prompt         string
	PreviousAnswer string
}

// SemanticInput is one task and, when present, its explicitly supplied
// following conversation turn. Conversation text is sent to the Judge but is
// never serialized into semantic cache values.
type SemanticInput struct {
	TurnID   string
	Prompt   string
	Followup *SemanticFollowup
}

// NewSemanticInput converts existing session data without exposing session
// types in the semantic result representation.
func NewSemanticInput(task sessions.Interaction, followup *sessions.Followup) SemanticInput {
	input := SemanticInput{TurnID: task.TurnID, Prompt: task.Prompt}
	if followup != nil {
		input.Followup = &SemanticFollowup{
			TurnID:         followup.TurnID,
			Prompt:         followup.Prompt,
			PreviousAnswer: followup.PreviousAnswer,
		}
	}
	return input
}

// BuildSemanticInputs prepares one semantic input per task that has usable
// prompt text or an explicitly supplied follow-up. Empty prompt/no-follow-up
// interactions are intentionally omitted.
func BuildSemanticInputs(interactions []sessions.Interaction, followups []sessions.Followup) []SemanticInput {
	byPrevious := make(map[string]*sessions.Followup, len(followups))
	for i := range followups {
		followup := followups[i]
		byPrevious[followup.PreviousTurnID] = &followup
	}

	result := make([]SemanticInput, 0, len(interactions))
	for _, task := range interactions {
		followup := byPrevious[task.TurnID]
		if task.Prompt == "" && followup == nil {
			continue
		}
		result = append(result, NewSemanticInput(task, followup))
	}
	return result
}

// SemanticResult contains only enum labels and numeric confidence values.
// Empty TaskType means task classification was unavailable because the input
// prompt was empty. Other empty optional fields mean that their classification
// does not apply to this record.
type SemanticResult struct {
	TurnID string `json:"turn_id"`

	TaskType       string  `json:"task_type"`
	TaskConfidence float64 `json:"task_confidence"`

	FollowupLabel      string  `json:"followup_label,omitempty"`
	FollowupConfidence float64 `json:"followup_confidence,omitempty"`

	SteeringReason           string  `json:"steering_reason,omitempty"`
	SteeringReasonConfidence float64 `json:"steering_reason_confidence,omitempty"`

	PreventionMechanisms []string `json:"prevention_mechanisms,omitempty"`
	NotPreventable       bool     `json:"not_preventable,omitempty"`
	PreventionConfidence float64  `json:"prevention_confidence,omitempty"`

	PromptIssue           string  `json:"prompt_issue,omitempty"`
	PromptIssueConfidence float64 `json:"prompt_issue_confidence,omitempty"`
	AgentsRule            string  `json:"agents_rule,omitempty"`
	AgentsRuleConfidence  float64 `json:"agents_rule_confidence,omitempty"`
	SkillCandidate        string  `json:"skill_candidate,omitempty"`
	SkillConfidence       float64 `json:"skill_confidence,omitempty"`
	ValidationType        string  `json:"validation_type,omitempty"`
	ValidationConfidence  float64 `json:"validation_confidence,omitempty"`
}

func (r SemanticResult) followupID() string {
	return r.TurnID
}

// ToTaskTypeResult and the other conversion methods are deterministic views
// used by the existing report code when semantic analysis is wired in.
func (r SemanticResult) ToTaskTypeResult() TaskTypeResult {
	return TaskTypeResult{TurnID: r.TurnID, Type: r.TaskType, Confidence: r.TaskConfidence}
}

func (r SemanticResult) ToTaskTypeResultIfAvailable() (TaskTypeResult, bool) {
	if r.TaskType == "" {
		return TaskTypeResult{}, false
	}
	return r.ToTaskTypeResult(), true
}

func (r SemanticResult) HasTaskType() bool { return r.TaskType != "" }

func (r SemanticResult) ToSteeringResult() (SteeringResult, bool) {
	if r.FollowupLabel == "" {
		return SteeringResult{}, false
	}
	return SteeringResult{PreviousTurnID: r.followupID(), Label: r.FollowupLabel, Confidence: r.FollowupConfidence}, true
}

func (r SemanticResult) ToSteeringReasonResult() (SteeringReasonResult, bool) {
	if r.SteeringReason == "" {
		return SteeringReasonResult{}, false
	}
	return SteeringReasonResult{PreviousTurnID: r.followupID(), Reason: r.SteeringReason, Confidence: r.SteeringReasonConfidence}, true
}

func (r SemanticResult) ToPreventionResult() (PreventionResult, bool) {
	if r.FollowupLabel != "steering" {
		return PreventionResult{}, false
	}
	return PreventionResult{PreviousTurnID: r.followupID(), Applicable: append([]string(nil), r.PreventionMechanisms...), NotPreventable: r.NotPreventable, Confidence: r.PreventionConfidence}, true
}

func (r SemanticResult) ToPromptQualityResult() (PromptQualityResult, bool) {
	if r.PromptIssue == "" {
		return PromptQualityResult{}, false
	}
	return PromptQualityResult{PreviousTurnID: r.followupID(), Issue: r.PromptIssue, Confidence: r.PromptIssueConfidence}, true
}

func (r SemanticResult) ToAgentsRuleResult() (AgentsRuleResult, bool) {
	if r.AgentsRule == "" {
		return AgentsRuleResult{}, false
	}
	return AgentsRuleResult{PreviousTurnID: r.followupID(), Rule: r.AgentsRule, Confidence: r.AgentsRuleConfidence}, true
}

func (r SemanticResult) ToSkillCandidateResult() (SkillCandidateResult, bool) {
	if r.SkillCandidate == "" {
		return SkillCandidateResult{}, false
	}
	return SkillCandidateResult{PreviousTurnID: r.followupID(), Category: r.SkillCandidate, Confidence: r.SkillConfidence}, true
}

func (r SemanticResult) ToValidationResult() (ValidationResult, bool) {
	if r.ValidationType == "" {
		return ValidationResult{}, false
	}
	return ValidationResult{PreviousTurnID: r.followupID(), ValidationType: r.ValidationType, Confidence: r.ValidationConfidence}, true
}

// Accessor aliases make it convenient for callers that prefer Get-style
// naming while retaining the explicit conversion methods above.
func (r SemanticResult) TaskTypeResult() TaskTypeResult         { return r.ToTaskTypeResult() }
func (r SemanticResult) SteeringResult() (SteeringResult, bool) { return r.ToSteeringResult() }
func (r SemanticResult) SteeringReasonResult() (SteeringReasonResult, bool) {
	return r.ToSteeringReasonResult()
}
func (r SemanticResult) PreventionResult() (PreventionResult, bool) { return r.ToPreventionResult() }
func (r SemanticResult) PromptQualityResult() (PromptQualityResult, bool) {
	return r.ToPromptQualityResult()
}
func (r SemanticResult) AgentsRuleResult() (AgentsRuleResult, bool) { return r.ToAgentsRuleResult() }
func (r SemanticResult) SkillCandidateResult() (SkillCandidateResult, bool) {
	return r.ToSkillCandidateResult()
}
func (r SemanticResult) ValidationResult() (ValidationResult, bool) { return r.ToValidationResult() }

type SemanticStats struct {
	TotalRecords      int
	CompletedRecords  int
	CacheHits         int
	EvaluatedRecords  int
	EvaluatedBatches  int
	ConfiguredWorkers int
	BatchSize         int
	Elapsed           time.Duration
	Methodology       string
	PromptVersion     string
	SchemaVersion     string
	Model             string
	Effort            string
}

type SemanticAnalysis struct {
	Results []SemanticResult
	Stats   SemanticStats
}

type SemanticRunner interface {
	Run(prompt string, schema any, result any) error
}

type SemanticConfig struct {
	Runner             SemanticRunner
	Cache              *analysiscache.Store
	Workers            int
	Concurrency        int
	BatchSize          int
	InitialBatchSize   int
	MaxRetries         int
	RetryBackoff       func(attempt int) time.Duration
	Model              string
	Effort             string
	MethodologyVersion string
	PromptVersion      string
	SchemaVersion      string
	Progress           func(SemanticProgress)
	OnProgress         func(SemanticProgress)
}

type SemanticProgress = SemanticStats

func DefaultSemanticConfig() SemanticConfig {
	runner := judge.New()
	return SemanticConfig{
		Runner: runner, Workers: SemanticDefaultWorkers, BatchSize: SemanticDefaultBatchSize,
		MaxRetries: 2, RetryBackoff: defaultSemanticBackoff,
		Model: runner.Model, Effort: runner.Effort,
		MethodologyVersion: SemanticMethodologyVersion, PromptVersion: SemanticPromptVersion, SchemaVersion: SemanticSchemaVersion,
	}
}

func defaultSemanticBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second * time.Duration(1<<(attempt-1))
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

type SemanticEngine struct{ config SemanticConfig }

func NewSemanticEngine(config SemanticConfig) *SemanticEngine {
	defaults := DefaultSemanticConfig()
	if config.Runner == nil {
		config.Runner = defaults.Runner
	}
	if config.Workers <= 0 && config.Concurrency > 0 {
		config.Workers = config.Concurrency
	}
	if config.Workers <= 0 {
		config.Workers = defaults.Workers
	}
	if config.BatchSize <= 0 && config.InitialBatchSize > 0 {
		config.BatchSize = config.InitialBatchSize
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaults.BatchSize
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = defaults.MaxRetries
	}
	if config.RetryBackoff == nil {
		config.RetryBackoff = defaults.RetryBackoff
	}
	if config.Model == "" {
		config.Model = defaults.Model
	}
	if config.Effort == "" {
		config.Effort = defaults.Effort
	}
	if config.MethodologyVersion == "" {
		config.MethodologyVersion = defaults.MethodologyVersion
	}
	if config.PromptVersion == "" {
		config.PromptVersion = defaults.PromptVersion
	}
	if config.SchemaVersion == "" {
		config.SchemaVersion = defaults.SchemaVersion
	}
	return &SemanticEngine{config: config}
}

func NewSemanticAnalyzer(config SemanticConfig) *SemanticEngine { return NewSemanticEngine(config) }

func NewDefaultSemanticEngine() (*SemanticEngine, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return nil, err
	}
	config := DefaultSemanticConfig()
	config.Cache = store
	return NewSemanticEngine(config), nil
}

func (e *SemanticEngine) AnalyzeInteractions(interactions []sessions.Interaction, followups []sessions.Followup) (SemanticAnalysis, error) {
	return e.Analyze(BuildSemanticInputs(interactions, followups))
}

type semanticIndexedInput struct {
	index int
	input SemanticInput
}

type semanticJudgeResponse struct {
	Results []semanticJudgeResult `json:"results"`
}
type semanticJudgeResult struct {
	TurnID                   string   `json:"turn_id"`
	TaskType                 *string  `json:"task_type"`
	TaskConfidence           *float64 `json:"task_confidence"`
	FollowupLabel            *string  `json:"followup_label"`
	FollowupConfidence       *float64 `json:"followup_confidence"`
	SteeringReason           *string  `json:"steering_reason"`
	SteeringReasonConfidence *float64 `json:"steering_reason_confidence"`
	PreventionMechanisms     []string `json:"prevention_mechanisms"`
	NotPreventable           *bool    `json:"not_preventable"`
	PreventionConfidence     *float64 `json:"prevention_confidence"`
	PromptIssue              *string  `json:"prompt_issue"`
	PromptIssueConfidence    *float64 `json:"prompt_issue_confidence"`
	AgentsRule               *string  `json:"agents_rule"`
	AgentsRuleConfidence     *float64 `json:"agents_rule_confidence"`
	SkillCandidate           *string  `json:"skill_candidate"`
	SkillConfidence          *float64 `json:"skill_confidence"`
	ValidationType           *string  `json:"validation_type"`
	ValidationConfidence     *float64 `json:"validation_confidence"`
}

func validateSemanticJudgePresence(input SemanticInput, result semanticJudgeResult) error {
	if input.Prompt == "" {
		if result.TaskType != nil || result.TaskConfidence != nil {
			return fmt.Errorf("turn %q has task classification despite unavailable prompt", input.TurnID)
		}
	} else {
		if result.TaskType == nil || *result.TaskType == "" || result.TaskConfidence == nil {
			return fmt.Errorf("turn %q requires task type and confidence", input.TurnID)
		}
	}
	label := ""
	if result.FollowupLabel != nil {
		label = *result.FollowupLabel
	}
	if input.Followup == nil {
		if result.NotPreventable == nil {
			return fmt.Errorf("turn %q is missing required not_preventable field", input.TurnID)
		}
		if label != "" || result.FollowupConfidence != nil || result.SteeringReason != nil || result.SteeringReasonConfidence != nil || len(result.PreventionMechanisms) != 0 || *result.NotPreventable || result.PreventionConfidence != nil || result.PromptIssue != nil || result.PromptIssueConfidence != nil || result.AgentsRule != nil || result.AgentsRuleConfidence != nil || result.SkillCandidate != nil || result.SkillConfidence != nil || result.ValidationType != nil || result.ValidationConfidence != nil {
			return fmt.Errorf("turn %q has non-null follow-up fields without a follow-up", input.TurnID)
		}
		return nil
	}
	if result.FollowupLabel == nil || label == "" || result.FollowupConfidence == nil {
		return fmt.Errorf("turn %q requires follow-up label and confidence", input.TurnID)
	}
	if label != "steering" {
		if result.NotPreventable == nil {
			return fmt.Errorf("turn %q is missing required not_preventable field", input.TurnID)
		}
		if result.SteeringReason != nil || result.SteeringReasonConfidence != nil || len(result.PreventionMechanisms) != 0 || *result.NotPreventable || result.PreventionConfidence != nil || result.PromptIssue != nil || result.PromptIssueConfidence != nil || result.AgentsRule != nil || result.AgentsRuleConfidence != nil || result.SkillCandidate != nil || result.SkillConfidence != nil || result.ValidationType != nil || result.ValidationConfidence != nil {
			return fmt.Errorf("turn %q has non-null steering-only fields for non-steering follow-up", input.TurnID)
		}
		return nil
	}
	if result.SteeringReason == nil || *result.SteeringReason == "" || result.SteeringReasonConfidence == nil {
		return fmt.Errorf("turn %q requires steering reason and confidence", input.TurnID)
	}
	if result.NotPreventable == nil {
		return fmt.Errorf("turn %q requires not_preventable", input.TurnID)
	}
	if len(result.PreventionMechanisms) == 0 {
		if *result.NotPreventable && result.PreventionConfidence != nil {
			return fmt.Errorf("turn %q has prevention confidence without mechanisms", input.TurnID)
		}
		if !*result.NotPreventable {
			return fmt.Errorf("turn %q must mark no mechanisms as not preventable", input.TurnID)
		}
	} else if result.PreventionConfidence == nil {
		return fmt.Errorf("turn %q requires prevention confidence with mechanisms", input.TurnID)
	}
	for _, conditional := range []struct {
		mechanism  string
		value      *string
		confidence *float64
	}{
		{"task_prompt", result.PromptIssue, result.PromptIssueConfidence},
		{"agents_md", result.AgentsRule, result.AgentsRuleConfidence},
		{"skill", result.SkillCandidate, result.SkillConfidence},
		{"validation", result.ValidationType, result.ValidationConfidence},
	} {
		applies := containsString(result.PreventionMechanisms, conditional.mechanism)
		if applies && (conditional.value == nil || *conditional.value == "" || conditional.confidence == nil) {
			return fmt.Errorf("turn %q requires %s classification and confidence", input.TurnID, conditional.mechanism)
		}
		if !applies && (conditional.value != nil || conditional.confidence != nil) {
			return fmt.Errorf("turn %q has %s classification without its prevention mechanism", input.TurnID, conditional.mechanism)
		}
	}
	return nil
}

func semanticResultFromJudge(result semanticJudgeResult) SemanticResult {
	out := SemanticResult{TurnID: result.TurnID, PreventionMechanisms: append([]string(nil), result.PreventionMechanisms...)}
	if result.TaskType != nil {
		out.TaskType = *result.TaskType
	}
	if result.TaskConfidence != nil {
		out.TaskConfidence = *result.TaskConfidence
	}
	if result.FollowupLabel != nil {
		out.FollowupLabel = *result.FollowupLabel
	}
	if result.FollowupConfidence != nil {
		out.FollowupConfidence = *result.FollowupConfidence
	}
	if result.SteeringReason != nil {
		out.SteeringReason = *result.SteeringReason
	}
	if result.SteeringReasonConfidence != nil {
		out.SteeringReasonConfidence = *result.SteeringReasonConfidence
	}
	if result.NotPreventable != nil {
		out.NotPreventable = *result.NotPreventable
	}
	if result.PreventionConfidence != nil {
		out.PreventionConfidence = *result.PreventionConfidence
	}
	if result.PromptIssue != nil {
		out.PromptIssue = *result.PromptIssue
	}
	if result.PromptIssueConfidence != nil {
		out.PromptIssueConfidence = *result.PromptIssueConfidence
	}
	if result.AgentsRule != nil {
		out.AgentsRule = *result.AgentsRule
	}
	if result.AgentsRuleConfidence != nil {
		out.AgentsRuleConfidence = *result.AgentsRuleConfidence
	}
	if result.SkillCandidate != nil {
		out.SkillCandidate = *result.SkillCandidate
	}
	if result.SkillConfidence != nil {
		out.SkillConfidence = *result.SkillConfidence
	}
	if result.ValidationType != nil {
		out.ValidationType = *result.ValidationType
	}
	if result.ValidationConfidence != nil {
		out.ValidationConfidence = *result.ValidationConfidence
	}
	return out
}

type semanticInvalidBatchError struct{ err error }

func (e *semanticInvalidBatchError) Error() string { return e.err.Error() }
func (e *semanticInvalidBatchError) Unwrap() error { return e.err }

func (e *SemanticEngine) Analyze(inputs []SemanticInput) (SemanticAnalysis, error) {
	started := time.Now()
	stats := SemanticStats{TotalRecords: len(inputs), ConfiguredWorkers: e.config.Workers, BatchSize: e.config.BatchSize, Methodology: e.config.MethodologyVersion, PromptVersion: e.config.PromptVersion, SchemaVersion: e.config.SchemaVersion, Model: e.config.Model, Effort: e.config.Effort}
	if err := validateSemanticInputs(inputs); err != nil {
		return SemanticAnalysis{}, err
	}
	results := make([]SemanticResult, len(inputs))
	pending := make([]semanticIndexedInput, 0, len(inputs))
	for i, input := range inputs {
		var cached SemanticResult
		if e.config.Cache != nil && e.config.Cache.Get(semanticCacheKey(input, e.config), &cached) && cached.TurnID == input.TurnID && ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{cached}) == nil {
			results[i] = cached
			stats.CacheHits++
			continue
		}
		pending = append(pending, semanticIndexedInput{index: i, input: input})
	}
	var progressMu sync.Mutex
	emitProgress := func(evaluatedRecords, evaluatedBatches int) {
		callback := e.config.Progress
		if callback == nil {
			callback = e.config.OnProgress
		}
		if callback == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		callback(SemanticProgress{
			TotalRecords: len(inputs), CompletedRecords: stats.CacheHits + evaluatedRecords,
			CacheHits: stats.CacheHits, EvaluatedRecords: evaluatedRecords, EvaluatedBatches: evaluatedBatches,
			ConfiguredWorkers: stats.ConfiguredWorkers, BatchSize: stats.BatchSize, Elapsed: time.Since(started),
			Methodology: stats.Methodology, PromptVersion: stats.PromptVersion, SchemaVersion: stats.SchemaVersion,
			Model: stats.Model, Effort: stats.Effort,
		})
	}
	// Report the cache lookup as the initial progress event, even when every
	// record was served from cache.
	emitProgress(0, 0)

	var evaluatedRecords atomic.Int64
	var evaluatedBatches atomic.Int64
	var mu sync.Mutex
	var firstErr error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := make(chan []semanticIndexedInput)
	var wg sync.WaitGroup
	workerCount := e.config.Workers
	if workerCount > len(pending) && len(pending) > 0 {
		workerCount = len(pending)
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var batch []semanticIndexedInput
				var ok bool
				select {
				case <-ctx.Done():
					return
				case batch, ok = <-jobs:
					if !ok {
						return
					}
				}
				if ctx.Err() != nil {
					return
				}
				if err := e.processSemanticBatch(batch, results, &mu, &evaluatedRecords, &evaluatedBatches, emitProgress); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
				}
			}
		}()
	}
dispatch:
	for start := 0; start < len(pending); start += e.config.BatchSize {
		end := min(start+e.config.BatchSize, len(pending))
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- pending[start:end]:
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return SemanticAnalysis{}, firstErr
	}
	stats.EvaluatedRecords = int(evaluatedRecords.Load())
	stats.EvaluatedBatches = int(evaluatedBatches.Load())
	stats.CompletedRecords = len(inputs)
	stats.Elapsed = time.Since(started)
	return SemanticAnalysis{Results: results, Stats: stats}, nil
}

func validateSemanticInputs(inputs []SemanticInput) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.TurnID == "" {
			return errors.New("semantic input has empty turn id")
		}
		if _, ok := seen[input.TurnID]; ok {
			return fmt.Errorf("duplicate semantic input turn id %q", input.TurnID)
		}
		if input.Followup != nil && strings.TrimSpace(input.Followup.Prompt) == "" {
			return fmt.Errorf("semantic input turn %q has blank follow-up prompt", input.TurnID)
		}
		seen[input.TurnID] = struct{}{}
	}
	return nil
}

func (e *SemanticEngine) processSemanticBatch(batch []semanticIndexedInput, output []SemanticResult, mu *sync.Mutex, evaluatedRecords, evaluatedBatches *atomic.Int64, emitProgress func(int, int)) error {
	inputs := make([]SemanticInput, len(batch))
	for i := range batch {
		inputs[i] = batch[i].input
	}
	results, err := e.runSemanticBatch(inputs)
	if err != nil {
		var invalid *semanticInvalidBatchError
		if errors.As(err, &invalid) && len(batch) > 1 {
			middle := len(batch) / 2
			if err := e.processSemanticBatch(batch[:middle], output, mu, evaluatedRecords, evaluatedBatches, emitProgress); err != nil {
				return fmt.Errorf("semantic split left: %w", err)
			}
			if err := e.processSemanticBatch(batch[middle:], output, mu, evaluatedRecords, evaluatedBatches, emitProgress); err != nil {
				return fmt.Errorf("semantic split right: %w", err)
			}
			return nil
		}
		return err
	}
	if err := ValidateSemanticResults(inputs, results); err != nil {
		return err
	}
	byID := make(map[string]SemanticResult, len(results))
	for _, result := range results {
		byID[result.TurnID] = result
	}
	if e.config.Cache != nil {
		for _, input := range inputs {
			if err := e.config.Cache.Set(semanticCacheKey(input, e.config), byID[input.TurnID]); err != nil {
				return fmt.Errorf("cache semantic result: %w", err)
			}
		}
		if err := e.config.Cache.Save(); err != nil {
			return fmt.Errorf("save semantic cache: %w", err)
		}
	}
	mu.Lock()
	for _, item := range batch {
		output[item.index] = byID[item.input.TurnID]
	}
	mu.Unlock()
	evaluatedRecords.Add(int64(len(batch)))
	evaluatedBatches.Add(1)
	emitProgress(int(evaluatedRecords.Load()), int(evaluatedBatches.Load()))
	return nil
}

func (e *SemanticEngine) runSemanticBatch(inputs []SemanticInput) ([]SemanticResult, error) {
	records := make([]map[string]any, len(inputs))
	for i, input := range inputs {
		record := map[string]any{"turn_id": input.TurnID, "prompt": input.Prompt}
		if input.Followup != nil {
			record["followup"] = map[string]string{"turn_id": input.Followup.TurnID, "prompt": input.Followup.Prompt, "previous_answer": input.Followup.PreviousAnswer}
		}
		records[i] = record
	}
	data, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("marshal semantic records: %w", err)
	}
	prompt := semanticJudgePrompt + "\n\nRecords:\n" + string(data)
	var lastErr error
	var firstValidationErr error
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(e.config.RetryBackoff(attempt))
		}
		var response semanticJudgeResponse
		if err := e.config.Runner.Run(prompt, semanticSchema(), &response); err != nil {
			lastErr = err
			if !isTransientSemanticError(err) || attempt == e.config.MaxRetries {
				return nil, err
			}
			continue
		}
		results := make([]SemanticResult, len(response.Results))
		expected := make(map[string]SemanticInput, len(inputs))
		for _, input := range inputs {
			expected[input.TurnID] = input
		}
		var validationErr error
		for i, result := range response.Results {
			input, ok := expected[result.TurnID]
			if !ok {
				validationErr = &semanticInvalidBatchError{err: fmt.Errorf("judge returned unexpected turn id %q", result.TurnID)}
				break
			}
			if err := validateSemanticJudgePresence(input, result); err != nil {
				validationErr = &semanticInvalidBatchError{err: err}
				break
			}
			results[i] = semanticResultFromJudge(result)
		}
		if validationErr == nil {
			if err := ValidateSemanticResults(inputs, results); err != nil {
				validationErr = &semanticInvalidBatchError{err: err}
			}
		}
		if validationErr != nil {
			if len(inputs) != 1 {
				return nil, validationErr
			}
			if firstValidationErr == nil {
				firstValidationErr = validationErr
			}
			if attempt == e.config.MaxRetries {
				return nil, firstValidationErr
			}
			continue
		}
		return results, nil
	}
	return nil, lastErr
}

func isTransientSemanticError(err error) bool {
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"rate limit", "ratelimit", "timeout", "temporar", "unavailable", "429", "500", "502", "503", "504", "provider"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func semanticCacheKey(input SemanticInput, config SemanticConfig) string {
	hash := sha256.New()
	for _, part := range []string{config.MethodologyVersion, config.PromptVersion, config.SchemaVersion, config.Model, config.Effort, input.TurnID, input.Prompt} {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	if input.Followup != nil {
		for _, part := range []string{input.Followup.TurnID, input.Followup.Prompt, input.Followup.PreviousAnswer} {
			hash.Write([]byte(part))
			hash.Write([]byte{0})
		}
	}
	return "semantic:" + hex.EncodeToString(hash.Sum(nil))
}

func SemanticCacheKey(input SemanticInput, config SemanticConfig) string {
	return semanticCacheKey(input, config)
}

func validateSemanticResults(inputs []SemanticInput, results []SemanticResult) error {
	expected := make(map[string]SemanticInput, len(inputs))
	for _, input := range inputs {
		expected[input.TurnID] = input
	}
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		input, ok := expected[result.TurnID]
		if !ok {
			return fmt.Errorf("judge returned unexpected turn id %q", result.TurnID)
		}
		if _, duplicate := seen[result.TurnID]; duplicate {
			return fmt.Errorf("judge returned duplicate turn id %q", result.TurnID)
		}
		seen[result.TurnID] = struct{}{}
		if err := validateSemanticRecord(input, result); err != nil {
			return err
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("judge returned %d results, expected %d", len(seen), len(expected))
	}
	return nil
}

func ValidateSemanticResults(inputs []SemanticInput, results []SemanticResult) error {
	return validateSemanticResults(inputs, results)
}

func validateSemanticRecord(input SemanticInput, result SemanticResult) error {
	if input.Prompt == "" {
		if result.TaskType != "" || result.TaskConfidence != 0 {
			return fmt.Errorf("turn %q has task classification despite unavailable prompt", input.TurnID)
		}
	} else if !isTaskType(result.TaskType) {
		return fmt.Errorf("judge returned unsupported task type %q", result.TaskType)
	}
	if input.Followup == nil {
		if hasFollowupFields(result) {
			return fmt.Errorf("turn %q has follow-up fields without a follow-up", input.TurnID)
		}
		return nil
	}
	if !isSteeringLabel(result.FollowupLabel) {
		return fmt.Errorf("judge returned unsupported steering label %q", result.FollowupLabel)
	}
	if result.FollowupLabel != "steering" {
		if hasSteeringFields(result) {
			return fmt.Errorf("turn %q has steering-only fields for non-steering follow-up", input.TurnID)
		}
		return nil
	}
	if !isSteeringReason(result.SteeringReason) {
		return fmt.Errorf("judge returned unsupported steering reason %q", result.SteeringReason)
	}
	if len(result.PreventionMechanisms) == 0 && !result.NotPreventable {
		return fmt.Errorf("turn %q has no prevention mechanism but is not marked not preventable", input.TurnID)
	}
	if result.NotPreventable && len(result.PreventionMechanisms) != 0 {
		return fmt.Errorf("turn %q is marked not preventable but has applicable mechanisms", input.TurnID)
	}
	if len(result.PreventionMechanisms) == 0 && result.PreventionConfidence != 0 {
		return fmt.Errorf("turn %q has prevention confidence without an applicable prevention mechanism", input.TurnID)
	}
	allowed := map[string]struct{}{"agents_md": {}, "skill": {}, "task_prompt": {}, "validation": {}}
	used := make(map[string]struct{}, len(result.PreventionMechanisms))
	for _, mechanism := range result.PreventionMechanisms {
		if _, ok := allowed[mechanism]; !ok {
			return fmt.Errorf("judge returned unsupported prevention mechanism %q", mechanism)
		}
		if _, ok := used[mechanism]; ok {
			return fmt.Errorf("turn %q has duplicate prevention mechanism %q", input.TurnID, mechanism)
		}
		used[mechanism] = struct{}{}
	}
	if err := validateConditional(result, "task_prompt", result.PromptIssue, result.PromptIssueConfidence, isPromptQualityIssue); err != nil {
		return err
	}
	if err := validateConditional(result, "agents_md", result.AgentsRule, result.AgentsRuleConfidence, isAgentsRule); err != nil {
		return err
	}
	if err := validateConditional(result, "skill", result.SkillCandidate, result.SkillConfidence, isSkillCandidate); err != nil {
		return err
	}
	if err := validateConditional(result, "validation", result.ValidationType, result.ValidationConfidence, isValidationType); err != nil {
		return err
	}
	return nil
}

func validateConditional(result SemanticResult, mechanism, value string, confidence float64, valid func(string) bool) error {
	present := containsString(result.PreventionMechanisms, mechanism)
	if present && !valid(value) {
		return fmt.Errorf("prevention mechanism %q requires a valid follow-on classification, got %q", mechanism, value)
	}
	if !present && (value != "" || confidence != 0) {
		return fmt.Errorf("follow-on classification %q is present without prevention mechanism %q", value, mechanism)
	}
	return nil
}

func hasFollowupFields(r SemanticResult) bool {
	return r.FollowupLabel != "" || r.FollowupConfidence != 0 || hasSteeringFields(r)
}
func hasSteeringFields(r SemanticResult) bool {
	return r.SteeringReason != "" || r.SteeringReasonConfidence != 0 || r.NotPreventable || len(r.PreventionMechanisms) != 0 || r.PreventionConfidence != 0 || r.PromptIssue != "" || r.PromptIssueConfidence != 0 || r.AgentsRule != "" || r.AgentsRuleConfidence != 0 || r.SkillCandidate != "" || r.SkillConfidence != 0 || r.ValidationType != "" || r.ValidationConfidence != 0
}

func isValidationType(value string) bool {
	switch value {
	case "tests", "static_analysis", "runtime_smoke_check", "deployment_check", "output_validation", "diff_review", "data_validation", "other":
		return true
	default:
		return false
	}
}

const semanticJudgePrompt = `Classify every supplied task exactly once using the unified semantic taxonomy. Use the primary intent and the supplied records only.

Task types (choose exactly one):
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

Follow-up labels (choose exactly one when a follow-up is supplied):
steering = user corrects or redirects Codex because previous work or understanding was wrong
continuation = normal next step or additional request
user_correction = user changes or corrects their own requirement, not a Codex mistake
question = user mainly asks a question or clarification

Steering reasons (choose exactly one for a steering follow-up):
misunderstood_request = Codex misunderstood what the user wanted
overengineering = Codex added unnecessary complexity or extra work
implementation_error = generated implementation was technically incorrect
ignored_constraints = Codex ignored an explicit requirement or limitation
insufficient_validation = Codex failed to verify, test, or check its work sufficiently
architecture_mismatch = solution did not fit project architecture or conventions
wrong_output_format = result was delivered in the wrong requested format
other = none of the above

Prevention mechanisms (choose only mechanisms that would have a high probability of preventing or automatically catching THIS specific failure):
agents_md = a stable project-wide rule would likely prevent the mistake
skill = a reusable workflow for this task type would likely prevent the mistake
task_prompt = missing or ambiguous information in THIS request materially caused the mistake
validation = an automated test, check, or review would likely catch the mistake before completion
Prefer 0-2 mechanisms per case; more than 2 is exceptional. If none clearly qualify, use prevention_mechanisms=[] and not_preventable=true. Otherwise use not_preventable=false. Do not mark a case not preventable when mechanisms are present.

Prompt issues (classify only when task_prompt applies; choose exactly one):
missing_constraints = important requirements or limitations were not stated
missing_context = necessary project, domain, or input context was not stated
ambiguous_request = the request allowed materially different interpretations
missing_acceptance = the desired completion or success criteria were not stated
missing_scope = boundaries of what to change or not change were not stated
wrong_assumption = the prompt contained or implied a materially wrong assumption
other = none of the above

AGENTS.md rules (classify only when agents_md applies; choose exactly one primary recurring project-level rule):
avoid_unnecessary_complexity = prefer the simplest sufficient implementation
preserve_project_architecture = fit existing structure, abstractions, and conventions
follow_explicit_constraints = obey stated requirements, limits, and prohibitions
avoid_scope_creep = change only the requested scope
inspect_existing_code_first = inspect relevant files and instructions before acting
validate_before_completion = run appropriate checks and verify the result before declaring completion
follow_requested_output_format = deliver the requested format or artifact
other = no stable project-level rule is a useful primary explanation
Use only rules that could be written as a recurring project instruction.

Skill candidates (classify only when skill applies; choose exactly one reusable workflow category):
implementation_prompt_file = a reusable workflow for turning a request into a precise implementation prompt file for another implementation worker
bounded_architecture_review = a reusable workflow for a focused architecture review with explicit boundaries and existing-code inspection
verification_workflow = a reusable workflow for systematic testing, validation, or artifact verification before completion
other = noise, an isolated domain-specific need, or no reusable workflow should be generalized
Generalize only workflows useful across multiple projects; use other for highly specific cases.

Validation types (classify only when validation applies; choose exactly one primary mechanism):
tests = unit, integration, end-to-end, regression, or behavioral tests
static_analysis = lint, type checking, compiler, PHPStan, or similar static checks
runtime_smoke_check = actually run the application, command, service, or feature and verify behavior
deployment_check = verify CI/CD, Kubernetes, deployment, infrastructure, monitoring, or post-deploy state
output_validation = inspect generated file, Markdown, PDF, artifact, format, or content
diff_review = review the produced code or diff for unintended changes, regressions, or scope creep
data_validation = verify SQL, API responses, calculations, metrics, or actual data
other = none of the above

Every task with a non-empty prompt has task_type and task_confidence. If the task prompt is empty/unavailable, task_type and task_confidence must be null; do not use other as a substitute. A task with no follow-up must use empty/null follow-up fields. Follow-up classification remains fully required whenever a follow-up is supplied, including when the task prompt is unavailable. Non-steering follow-ups must use empty/null steering-only and prevention/follow-on fields. A steering follow-up must provide steering_reason and either one or more prevention mechanisms or not_preventable=true. Each optional follow-on classification and its confidence must be present exactly when its prevention mechanism applies. Return every supplied input ID exactly once, with no missing, duplicate, or unexpected IDs. Return no prose and do not use tools.`

func semanticSchema() map[string]any {
	numberOrNull := map[string]any{"type": []string{"number", "null"}}
	nullableEnum := func(values []string) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "string", "enum": values}}}
	}
	properties := map[string]any{
		"turn_id":             map[string]any{"type": "string"},
		"task_type":           nullableEnum([]string{"bugfix", "feature", "refactor", "tests", "code_review", "architecture", "devops", "research", "documentation", "other"}),
		"task_confidence":     numberOrNull,
		"followup_label":      nullableEnum([]string{"steering", "continuation", "user_correction", "question"}),
		"followup_confidence": numberOrNull,
		"steering_reason":     nullableEnum([]string{"misunderstood_request", "overengineering", "implementation_error", "ignored_constraints", "insufficient_validation", "architecture_mismatch", "wrong_output_format", "other"}), "steering_reason_confidence": numberOrNull,
		// Keep this provider-compatible: duplicate mechanism rejection is
		// performed by ValidateSemanticResults below.
		"prevention_mechanisms": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"agents_md", "skill", "task_prompt", "validation"}}},
		"not_preventable":       map[string]any{"type": "boolean"}, "prevention_confidence": numberOrNull,
		"prompt_issue": nullableEnum([]string{"missing_constraints", "missing_context", "ambiguous_request", "missing_acceptance", "missing_scope", "wrong_assumption", "other"}), "prompt_issue_confidence": numberOrNull,
		"agents_rule": nullableEnum([]string{"avoid_unnecessary_complexity", "preserve_project_architecture", "follow_explicit_constraints", "avoid_scope_creep", "inspect_existing_code_first", "validate_before_completion", "follow_requested_output_format", "other"}), "agents_rule_confidence": numberOrNull,
		"skill_candidate": nullableEnum([]string{"implementation_prompt_file", "bounded_architecture_review", "verification_workflow", "other"}), "skill_confidence": numberOrNull,
		"validation_type": nullableEnum([]string{"tests", "static_analysis", "runtime_smoke_check", "deployment_check", "output_validation", "diff_review", "data_validation", "other"}), "validation_confidence": numberOrNull,
	}
	record := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"turn_id", "task_type", "task_confidence", "followup_label", "followup_confidence", "steering_reason", "steering_reason_confidence", "prevention_mechanisms", "not_preventable", "prevention_confidence", "prompt_issue", "prompt_issue_confidence", "agents_rule", "agents_rule_confidence", "skill_candidate", "skill_confidence", "validation_type", "validation_confidence"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"results": map[string]any{"type": "array", "items": record}},
		"required":             []string{"results"},
		"additionalProperties": false,
	}
}
