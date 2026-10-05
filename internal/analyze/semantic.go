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
	"unicode/utf8"

	analysiscache "github.com/axcherednikov/codex-insights/internal/cache"
	"github.com/axcherednikov/codex-insights/internal/judge"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

// These versions deliberately have independent lifecycles. Changing the
// methodology, prompt, or wire schema invalidates semantic cache entries
// without requiring a disk-format migration.
const (
	SemanticMethodologyVersion = "semantic-v2"
	SemanticPromptVersion      = "semantic-judge-v2"
	SemanticSchemaVersion      = "semantic-schema-v2"
	SemanticDefaultWorkers     = 6
	SemanticDefaultBatchSize   = 20
	semanticMaxBatchBytes      = 512 * 1024
	semanticMaxTextBytes       = 24 * 1024
	semanticDefaultMaxRetries  = 2
	semanticBackoffCap         = 5 * time.Second
	semanticBackoffExponent    = 1
	semanticBatchSplitDivisor  = 2
	semanticTruncationDivisor  = 2
	semanticJSONArrayBytes     = 2
	semanticTruncationLabel    = "[truncated for semantic analysis]"
	semanticTruncationMarker   = "\n..." + semanticTruncationLabel + "...\n"
	semanticRecordsPrefix      = "\n\nRecords:\n"
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
		MaxRetries: semanticDefaultMaxRetries, RetryBackoff: defaultSemanticBackoff,
		Model: runner.Model, Effort: runner.Effort,
		MethodologyVersion: SemanticMethodologyVersion, PromptVersion: SemanticPromptVersion, SchemaVersion: SemanticSchemaVersion,
	}
}

func defaultSemanticBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second * time.Duration(1<<(attempt-semanticBackoffExponent))
	if delay > semanticBackoffCap {
		return semanticBackoffCap
	}

	return delay
}

type SemanticEngine struct{ config SemanticConfig }

func NewSemanticEngine(config SemanticConfig) *SemanticEngine {
	defaults := DefaultSemanticConfig()
	applySemanticExecutionDefaults(&config, defaults)
	applySemanticModelDefaults(&config, defaults)

	return &SemanticEngine{config: config}
}

func applySemanticExecutionDefaults(config *SemanticConfig, defaults SemanticConfig) {
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
}

func applySemanticModelDefaults(config *SemanticConfig, defaults SemanticConfig) {
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
}

func NewSemanticAnalyzer(config SemanticConfig) *SemanticEngine { return NewSemanticEngine(config) }

func NewDefaultSemanticEngine() (*SemanticEngine, error) {
	store, err := analysiscache.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("open semantic cache: %w", err)
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
	if err := validateSemanticTaskPresence(input, result); err != nil {
		return err
	}

	return validateSemanticFollowupPresence(input, result)
}

func validateSemanticTaskPresence(input SemanticInput, result semanticJudgeResult) error {
	if input.Prompt == "" {
		if result.TaskType != nil || result.TaskConfidence != nil {
			return semanticTurnErrorMessage(errSemanticTaskWhenPromptMissing, input.TurnID, "has task classification despite unavailable prompt")
		}

		return nil
	}
	if result.TaskType == nil || *result.TaskType == "" || result.TaskConfidence == nil {
		return semanticTurnErrorMessage(errSemanticTaskAndConfidenceRequired, input.TurnID, "requires task type and confidence")
	}

	return nil
}

func validateSemanticFollowupPresence(input SemanticInput, result semanticJudgeResult) error {
	label := semanticJudgeString(result.FollowupLabel)
	if input.Followup == nil {
		return validateSemanticAbsentFollowupPresence(input.TurnID, label, result)
	}
	if result.FollowupLabel == nil || label == "" || result.FollowupConfidence == nil {
		return semanticTurnErrorMessage(errSemanticFollowupAndConfidenceRequired, input.TurnID, "requires follow-up label and confidence")
	}
	if label != "steering" {
		return validateSemanticNonSteeringPresence(input.TurnID, result)
	}

	return validateSemanticSteeringPresence(input.TurnID, result)
}

func semanticJudgeString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func validateSemanticAbsentFollowupPresence(turnID, label string, result semanticJudgeResult) error {
	if result.NotPreventable == nil {
		return semanticTurnErrorMessage(errSemanticNotPreventableMissing, turnID, "is missing required not_preventable field")
	}
	if hasSemanticJudgeFollowupFields(label, result) {
		return semanticTurnErrorMessage(errSemanticFollowupFieldsWithoutFollowup, turnID, "has non-null follow-up fields without a follow-up")
	}

	return nil
}

func hasSemanticJudgeFollowupFields(label string, result semanticJudgeResult) bool {
	return label != "" || result.FollowupConfidence != nil || hasSemanticJudgeSteeringFields(result)
}

func validateSemanticNonSteeringPresence(turnID string, result semanticJudgeResult) error {
	if result.NotPreventable == nil {
		return semanticTurnErrorMessage(errSemanticNotPreventableMissing, turnID, "is missing required not_preventable field")
	}
	if hasSemanticJudgeSteeringFields(result) {
		return semanticTurnErrorMessage(errSemanticSteeringFieldsForOtherLabel, turnID, "has non-null steering-only fields for non-steering follow-up")
	}

	return nil
}

func hasSemanticJudgeSteeringFields(result semanticJudgeResult) bool {
	return result.SteeringReason != nil || result.SteeringReasonConfidence != nil || len(result.PreventionMechanisms) != 0 ||
		*result.NotPreventable || result.PreventionConfidence != nil || result.PromptIssue != nil || result.PromptIssueConfidence != nil ||
		result.AgentsRule != nil || result.AgentsRuleConfidence != nil || result.SkillCandidate != nil ||
		result.SkillConfidence != nil || result.ValidationType != nil || result.ValidationConfidence != nil
}

func validateSemanticSteeringPresence(turnID string, result semanticJudgeResult) error {
	if result.SteeringReason == nil || *result.SteeringReason == "" || result.SteeringReasonConfidence == nil {
		return semanticTurnErrorMessage(errSemanticSteeringReasonAndConfidenceRequired, turnID, "requires steering reason and confidence")
	}
	if result.NotPreventable == nil {
		return semanticTurnErrorMessage(errSemanticNotPreventableRequired, turnID, "requires not_preventable")
	}
	if err := validateSemanticPresencePrevention(turnID, result); err != nil {
		return err
	}

	return validateSemanticConditionalPresence(turnID, result)
}

func validateSemanticPresencePrevention(turnID string, result semanticJudgeResult) error {
	if len(result.PreventionMechanisms) == 0 {
		if *result.NotPreventable && result.PreventionConfidence != nil {
			return semanticTurnErrorMessage(errSemanticPreventionConfidenceWithoutMechanism, turnID, "has prevention confidence without mechanisms")
		}
		if !*result.NotPreventable {
			return semanticTurnErrorMessage(errSemanticEmptyMechanismMustBeNotPreventable, turnID, "must mark no mechanisms as not preventable")
		}

		return nil
	}
	if result.PreventionConfidence == nil {
		return semanticTurnErrorMessage(errSemanticPreventionConfidenceRequired, turnID, "requires prevention confidence with mechanisms")
	}

	return nil
}

func validateSemanticConditionalPresence(turnID string, result semanticJudgeResult) error {
	for _, conditional := range semanticJudgeConditionals(result) {
		applies := containsString(result.PreventionMechanisms, conditional.mechanism)
		if applies && (conditional.value == nil || *conditional.value == "" || conditional.confidence == nil) {
			return fmt.Errorf("%w %q requires %s classification and confidence", errSemanticConditionalClassificationRequired, turnID, conditional.mechanism)
		}
		if !applies && (conditional.value != nil || conditional.confidence != nil) {
			return fmt.Errorf("%w %q has %s classification without its prevention mechanism", errSemanticClassificationWithoutMechanism, turnID, conditional.mechanism)
		}
	}

	return nil
}

type semanticJudgeConditional struct {
	mechanism  string
	value      *string
	confidence *float64
}

func semanticJudgeConditionals(result semanticJudgeResult) []semanticJudgeConditional {
	return []semanticJudgeConditional{
		{mechanism: "task_prompt", value: result.PromptIssue, confidence: result.PromptIssueConfidence},
		{mechanism: "agents_md", value: result.AgentsRule, confidence: result.AgentsRuleConfidence},
		{mechanism: "skill", value: result.SkillCandidate, confidence: result.SkillConfidence},
		{mechanism: "validation", value: result.ValidationType, confidence: result.ValidationConfidence},
	}
}

func semanticTurnErrorMessage(identity semanticTurnError, turnID, message string) error {
	return fmt.Errorf("%w %q %s", identity, turnID, message)
}

func semanticResultFromJudge(result semanticJudgeResult) SemanticResult {
	out := SemanticResult{
		TurnID:               result.TurnID,
		PreventionMechanisms: append([]string(nil), result.PreventionMechanisms...),
	}
	copySemanticTaskFields(&out, result)
	copySemanticFollowupFields(&out, result)
	copySemanticSteeringFields(&out, result)
	copySemanticPreventionFields(&out, result)
	copySemanticClassificationFields(&out, result)

	return out
}

func copySemanticTaskFields(out *SemanticResult, result semanticJudgeResult) {
	if result.TaskType != nil {
		out.TaskType = *result.TaskType
	}
	if result.TaskConfidence != nil {
		out.TaskConfidence = *result.TaskConfidence
	}
}

func copySemanticFollowupFields(out *SemanticResult, result semanticJudgeResult) {
	if result.FollowupLabel != nil {
		out.FollowupLabel = *result.FollowupLabel
	}
	if result.FollowupConfidence != nil {
		out.FollowupConfidence = *result.FollowupConfidence
	}
}

func copySemanticSteeringFields(out *SemanticResult, result semanticJudgeResult) {
	if result.SteeringReason != nil {
		out.SteeringReason = *result.SteeringReason
	}
	if result.SteeringReasonConfidence != nil {
		out.SteeringReasonConfidence = *result.SteeringReasonConfidence
	}
}

func copySemanticPreventionFields(out *SemanticResult, result semanticJudgeResult) {
	if result.NotPreventable != nil {
		out.NotPreventable = *result.NotPreventable
	}
	if result.PreventionConfidence != nil {
		out.PreventionConfidence = *result.PreventionConfidence
	}
}

func copySemanticClassificationFields(out *SemanticResult, result semanticJudgeResult) {
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
}

type semanticInvalidBatchError struct{ err error }

func (e *semanticInvalidBatchError) Error() string { return e.err.Error() }
func (e *semanticInvalidBatchError) Unwrap() error { return e.err }

func (e *SemanticEngine) Analyze(inputs []SemanticInput) (SemanticAnalysis, error) {
	started := time.Now()
	stats := e.semanticStats(len(inputs))
	if err := validateSemanticInputs(inputs); err != nil {
		return SemanticAnalysis{}, err
	}

	results := make([]SemanticResult, len(inputs))
	pending := e.selectSemanticCache(inputs, results, &stats)
	batches, err := planSemanticBatches(pending, e.config.BatchSize, semanticMaxBatchBytes)
	if err != nil {
		return SemanticAnalysis{}, err
	}
	emitProgress := e.semanticProgressReporter(len(inputs), started, stats)
	// Report the cache lookup as the initial progress event, even when every
	// record was served from cache.
	emitProgress(0, 0)
	evaluatedRecords, evaluatedBatches, err := e.executeSemanticBatches(batches, results, emitProgress)
	if err != nil {
		return SemanticAnalysis{}, err
	}
	stats.EvaluatedRecords = evaluatedRecords
	stats.EvaluatedBatches = evaluatedBatches
	stats.CompletedRecords = len(inputs)
	stats.Elapsed = time.Since(started)

	return SemanticAnalysis{Results: results, Stats: stats}, nil
}

func (e *SemanticEngine) semanticStats(total int) SemanticStats {
	return SemanticStats{
		TotalRecords: total, ConfiguredWorkers: e.config.Workers, BatchSize: e.config.BatchSize,
		Methodology: e.config.MethodologyVersion, PromptVersion: e.config.PromptVersion,
		SchemaVersion: e.config.SchemaVersion, Model: e.config.Model, Effort: e.config.Effort,
	}
}

func (e *SemanticEngine) selectSemanticCache(
	inputs []SemanticInput,
	results []SemanticResult,
	stats *SemanticStats,
) []semanticIndexedInput {
	pending := make([]semanticIndexedInput, 0, len(inputs))
	for index, input := range inputs {
		if result, ok := e.cachedSemanticResult(input); ok {
			results[index] = result
			stats.CacheHits++

			continue
		}
		pending = append(pending, semanticIndexedInput{index: index, input: input})
	}

	return pending
}

func (e *SemanticEngine) cachedSemanticResult(input SemanticInput) (SemanticResult, bool) {
	var cached SemanticResult
	if e.config.Cache == nil || !e.config.Cache.Get(semanticCacheKey(input, e.config), &cached) || cached.TurnID != input.TurnID {
		return SemanticResult{}, false
	}
	if ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{cached}) != nil {
		return SemanticResult{}, false
	}

	return cached, true
}

func (e *SemanticEngine) semanticProgressReporter(
	total int,
	started time.Time,
	stats SemanticStats,
) func(int, int) {
	var progressMu sync.Mutex

	return func(evaluatedRecords, evaluatedBatches int) {
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
			TotalRecords: total, CompletedRecords: stats.CacheHits + evaluatedRecords,
			CacheHits: stats.CacheHits, EvaluatedRecords: evaluatedRecords, EvaluatedBatches: evaluatedBatches,
			ConfiguredWorkers: stats.ConfiguredWorkers, BatchSize: stats.BatchSize, Elapsed: time.Since(started),
			Methodology: stats.Methodology, PromptVersion: stats.PromptVersion, SchemaVersion: stats.SchemaVersion,
			Model: stats.Model, Effort: stats.Effort,
		})
	}
}

type semanticExecutionState struct {
	mu       sync.Mutex
	firstErr error
	cancel   context.CancelFunc
}

func (state *semanticExecutionState) record(err error) {
	state.mu.Lock()
	if state.firstErr == nil {
		state.firstErr = err
		state.cancel()
	}
	state.mu.Unlock()
}

func (state *semanticExecutionState) error() error {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.firstErr
}

func (e *SemanticEngine) executeSemanticBatches(
	batches [][]semanticIndexedInput,
	results []SemanticResult,
	emitProgress func(int, int),
) (int, int, error) {
	var evaluatedRecords atomic.Int64
	var evaluatedBatches atomic.Int64
	var outputMu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &semanticExecutionState{cancel: cancel}
	jobs := make(chan []semanticIndexedInput)
	var workers sync.WaitGroup
	workerCount := semanticWorkerCount(e.config.Workers, len(batches))
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go e.runSemanticWorker(ctx, jobs, results, &outputMu, &evaluatedRecords, &evaluatedBatches, emitProgress, state, &workers)
	}
	dispatchSemanticBatches(ctx, jobs, batches)
	workers.Wait()

	return int(evaluatedRecords.Load()), int(evaluatedBatches.Load()), state.error()
}

func semanticWorkerCount(configured, batches int) int {
	if configured > batches && batches > 0 {
		return batches
	}

	return configured
}

func dispatchSemanticBatches(ctx context.Context, jobs chan<- []semanticIndexedInput, batches [][]semanticIndexedInput) {
dispatch:
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- batch:
		}
	}
	close(jobs)
}

func (e *SemanticEngine) runSemanticWorker(
	ctx context.Context,
	jobs <-chan []semanticIndexedInput,
	results []SemanticResult,
	outputMu *sync.Mutex,
	evaluatedRecords, evaluatedBatches *atomic.Int64,
	emitProgress func(int, int),
	state *semanticExecutionState,
	workers *sync.WaitGroup,
) {
	defer workers.Done()
	for {
		batch, ok := nextSemanticBatch(ctx, jobs)
		if !ok {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if err := e.processSemanticBatch(batch, results, outputMu, evaluatedRecords, evaluatedBatches, emitProgress); err != nil {
			state.record(err)
		}
	}
}

func nextSemanticBatch(ctx context.Context, jobs <-chan []semanticIndexedInput) ([]semanticIndexedInput, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case batch, ok := <-jobs:
		return batch, ok
	}
}

func planSemanticBatches(items []semanticIndexedInput, maxRecords, maxBytes int) ([][]semanticIndexedInput, error) {
	if maxRecords < 1 || maxBytes < 1 {
		return nil, errSemanticBatchLimits
	}
	recordSizes := make([]int, len(items))
	for i := range items {
		record, err := marshalSemanticRecord(items[i].input)
		if err != nil {
			return nil, err
		}
		recordSizes[i] = len(record)
	}
	baseBytes := len(semanticJudgePrompt) + len(semanticRecordsPrefix) + semanticJSONArrayBytes
	batches := make([][]semanticIndexedInput, 0, (len(items)+maxRecords-1)/maxRecords)
	for start, end := 0, 0; start < len(items); {
		end = start
		batchBytes := baseBytes
		for end < len(items) && end-start < maxRecords {
			recordBytes := recordSizes[end]
			if end > start {
				recordBytes++
			}
			if batchBytes+recordBytes > maxBytes {
				break
			}
			batchBytes += recordBytes
			end++
		}
		if end == start {
			return nil, fmt.Errorf("%w %q exceeds internal prompt budget after truncation", errSemanticInputBudgetExceeded, items[start].input.TurnID)
		}
		batches = append(batches, items[start:end])
		start = end
	}

	return batches, nil
}

func validateSemanticInputs(inputs []SemanticInput) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.TurnID == "" {
			return errEmptySemanticTurnID
		}
		if _, ok := seen[input.TurnID]; ok {
			return fmt.Errorf("%w %q", errDuplicateSemanticInputTurnID, input.TurnID)
		}
		if input.Followup != nil && strings.TrimSpace(input.Followup.Prompt) == "" {
			return fmt.Errorf("%w %q has blank follow-up prompt", errBlankSemanticFollowupPrompt, input.TurnID)
		}
		seen[input.TurnID] = struct{}{}
	}

	return nil
}

func (e *SemanticEngine) processSemanticBatch(
	batch []semanticIndexedInput,
	output []SemanticResult,
	outputMu *sync.Mutex,
	evaluatedRecords, evaluatedBatches *atomic.Int64,
	emitProgress func(int, int),
) error {
	inputs := semanticBatchInputs(batch)
	results, err := e.runSemanticBatch(inputs)
	if err != nil {
		return e.handleSemanticBatchFailure(batch, err, output, outputMu, evaluatedRecords, evaluatedBatches, emitProgress)
	}
	if err := ValidateSemanticResults(inputs, results); err != nil {
		return err
	}

	return e.finalizeSemanticBatch(batch, inputs, results, output, outputMu, evaluatedRecords, evaluatedBatches, emitProgress)
}

func semanticBatchInputs(batch []semanticIndexedInput) []SemanticInput {
	inputs := make([]SemanticInput, len(batch))
	for index := range batch {
		inputs[index] = batch[index].input
	}

	return inputs
}

func (e *SemanticEngine) handleSemanticBatchFailure(
	batch []semanticIndexedInput,
	cause error,
	output []SemanticResult,
	outputMu *sync.Mutex,
	evaluatedRecords, evaluatedBatches *atomic.Int64,
	emitProgress func(int, int),
) error {
	if isSplittableSemanticBatchError(cause) && len(batch) > 1 {
		return e.processSemanticSplit(batch, output, outputMu, evaluatedRecords, evaluatedBatches, emitProgress)
	}
	if isSemanticInputTooLargeError(cause) {
		return fmt.Errorf("semantic input turn %q exceeds Judge input limit after truncation: %w", batch[0].input.TurnID, cause)
	}

	return cause
}

func (e *SemanticEngine) processSemanticSplit(
	batch []semanticIndexedInput,
	output []SemanticResult,
	outputMu *sync.Mutex,
	evaluatedRecords, evaluatedBatches *atomic.Int64,
	emitProgress func(int, int),
) error {
	middle := len(batch) / semanticBatchSplitDivisor
	if err := e.processSemanticBatch(batch[:middle], output, outputMu, evaluatedRecords, evaluatedBatches, emitProgress); err != nil {
		return fmt.Errorf("semantic split left: %w", err)
	}
	if err := e.processSemanticBatch(batch[middle:], output, outputMu, evaluatedRecords, evaluatedBatches, emitProgress); err != nil {
		return fmt.Errorf("semantic split right: %w", err)
	}

	return nil
}

func (e *SemanticEngine) finalizeSemanticBatch(
	batch []semanticIndexedInput,
	inputs []SemanticInput,
	results []SemanticResult,
	output []SemanticResult,
	outputMu *sync.Mutex,
	evaluatedRecords, evaluatedBatches *atomic.Int64,
	emitProgress func(int, int),
) error {
	byID := semanticResultsByID(results)
	if err := e.persistSemanticResults(inputs, byID); err != nil {
		return err
	}
	storeSemanticBatchResults(batch, byID, output, outputMu)
	evaluatedRecords.Add(int64(len(batch)))
	evaluatedBatches.Add(1)
	emitProgress(int(evaluatedRecords.Load()), int(evaluatedBatches.Load()))

	return nil
}

func semanticResultsByID(results []SemanticResult) map[string]SemanticResult {
	byID := make(map[string]SemanticResult, len(results))
	for _, result := range results {
		byID[result.TurnID] = result
	}

	return byID
}

func (e *SemanticEngine) persistSemanticResults(inputs []SemanticInput, results map[string]SemanticResult) error {
	if e.config.Cache == nil {
		return nil
	}
	for _, input := range inputs {
		if err := e.config.Cache.Set(semanticCacheKey(input, e.config), results[input.TurnID]); err != nil {
			return fmt.Errorf("cache semantic result: %w", err)
		}
	}
	if err := e.config.Cache.Save(); err != nil {
		return fmt.Errorf("save semantic cache: %w", err)
	}

	return nil
}

func storeSemanticBatchResults(
	batch []semanticIndexedInput,
	byID map[string]SemanticResult,
	output []SemanticResult,
	outputMu *sync.Mutex,
) {
	outputMu.Lock()
	for _, item := range batch {
		output[item.index] = byID[item.input.TurnID]
	}
	outputMu.Unlock()
}

func (e *SemanticEngine) runSemanticBatch(inputs []SemanticInput) ([]SemanticResult, error) {
	prompt, err := buildSemanticPrompt(inputs)
	if err != nil {
		return nil, err
	}
	var lastErr error
	var firstValidationErr error
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(e.config.RetryBackoff(attempt))
		}
		results, err := e.runSemanticAttempt(prompt, inputs)
		if err == nil {
			return results, nil
		}
		if isSemanticInvalidBatch(err) {
			if len(inputs) != 1 {
				return nil, err
			}
			if firstValidationErr == nil {
				firstValidationErr = err
			}
			if attempt == e.config.MaxRetries {
				return nil, firstValidationErr
			}

			continue
		}
		lastErr = err
		if isSemanticInputTooLargeError(err) || !isTransientSemanticError(err) || attempt == e.config.MaxRetries {
			return nil, err
		}
	}

	return nil, lastErr
}

func (e *SemanticEngine) runSemanticAttempt(prompt string, inputs []SemanticInput) ([]SemanticResult, error) {
	var response semanticJudgeResponse
	if err := e.config.Runner.Run(prompt, semanticSchema(), &response); err != nil {
		return nil, fmt.Errorf("run semantic Judge attempt: %w", err)
	}
	results, err := decodeSemanticResults(inputs, response.Results)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func decodeSemanticResults(inputs []SemanticInput, decoded []semanticJudgeResult) ([]SemanticResult, error) {
	results := make([]SemanticResult, len(decoded))
	expected := make(map[string]SemanticInput, len(inputs))
	for _, input := range inputs {
		expected[input.TurnID] = input
	}
	for index, result := range decoded {
		input, ok := expected[result.TurnID]
		if !ok {
			cause := fmt.Errorf("%w %q", errUnexpectedResultID, result.TurnID)

			return nil, &semanticInvalidBatchError{err: cause}
		}
		if err := validateSemanticJudgePresence(input, result); err != nil {
			return nil, &semanticInvalidBatchError{err: err}
		}
		results[index] = semanticResultFromJudge(result)
	}
	if err := ValidateSemanticResults(inputs, results); err != nil {
		return nil, &semanticInvalidBatchError{err: err}
	}

	return results, nil
}

func isSemanticInvalidBatch(err error) bool {
	var invalid *semanticInvalidBatchError

	return errors.As(err, &invalid)
}

func buildSemanticPrompt(inputs []SemanticInput) (string, error) {
	var prompt strings.Builder
	prompt.WriteString(semanticJudgePrompt)
	prompt.WriteString(semanticRecordsPrefix)
	prompt.WriteByte('[')
	for i, input := range inputs {
		record, err := marshalSemanticRecord(input)
		if err != nil {
			return "", err
		}
		if i > 0 {
			prompt.WriteByte(',')
		}
		prompt.Write(record)
	}
	prompt.WriteByte(']')

	return prompt.String(), nil
}

func marshalSemanticRecord(input SemanticInput) ([]byte, error) {
	record := map[string]any{"turn_id": input.TurnID, "prompt": truncateSemanticText(input.Prompt)}
	if input.Followup != nil {
		record["followup"] = map[string]string{
			"turn_id":         input.Followup.TurnID,
			"prompt":          truncateSemanticText(input.Followup.Prompt),
			"previous_answer": truncateSemanticText(input.Followup.PreviousAnswer),
		}
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal semantic record %q: %w", input.TurnID, err)
	}

	return data, nil
}

func truncateSemanticText(text string) string {
	if len(text) <= semanticMaxTextBytes {
		return text
	}
	contentBytes := semanticMaxTextBytes - len(semanticTruncationMarker)
	headBytes := contentBytes / semanticTruncationDivisor
	tailBytes := contentBytes - headBytes
	headEnd := headBytes
	for headEnd > 0 && !utf8.RuneStart(text[headEnd]) {
		headEnd--
	}
	tailStart := len(text) - tailBytes
	for tailStart < len(text) && !utf8.RuneStart(text[tailStart]) {
		tailStart++
	}

	return text[:headEnd] + semanticTruncationMarker + text[tailStart:]
}

func isSplittableSemanticBatchError(err error) bool {
	var invalid *semanticInvalidBatchError

	return errors.As(err, &invalid) || isSemanticInputTooLargeError(err)
}

func isSemanticInputTooLargeError(err error) bool {
	text := strings.ToLower(err.Error())

	return strings.Contains(text, "input_too_large") ||
		strings.Contains(text, "input exceeds the maximum length")
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
			return fmt.Errorf("%w %q", errUnexpectedResultID, result.TurnID)
		}
		if _, duplicate := seen[result.TurnID]; duplicate {
			return fmt.Errorf("%w %q", errDuplicateResultID, result.TurnID)
		}
		seen[result.TurnID] = struct{}{}
		if err := validateSemanticRecord(input, result); err != nil {
			return err
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w %d results, expected %d", errJudgeResultCount, len(seen), len(expected))
	}

	return nil
}

func ValidateSemanticResults(inputs []SemanticInput, results []SemanticResult) error {
	return validateSemanticResults(inputs, results)
}

func validateSemanticRecord(input SemanticInput, result SemanticResult) error {
	if err := validateSemanticTaskRecord(input, result); err != nil {
		return err
	}

	return validateSemanticFollowupRecord(input, result)
}

func validateSemanticTaskRecord(input SemanticInput, result SemanticResult) error {
	if input.Prompt == "" {
		if result.TaskType != "" || result.TaskConfidence != 0 {
			return semanticTurnErrorMessage(errSemanticTaskWhenPromptMissing, input.TurnID, "has task classification despite unavailable prompt")
		}

		return nil
	}
	if !isTaskType(result.TaskType) {
		return fmt.Errorf("%w %q", errUnsupportedTaskType, result.TaskType)
	}

	return nil
}

func validateSemanticFollowupRecord(input SemanticInput, result SemanticResult) error {
	if input.Followup == nil {
		if hasFollowupFields(result) {
			return semanticTurnErrorMessage(errSemanticFollowupFieldsWithoutFollowup, input.TurnID, "has follow-up fields without a follow-up")
		}

		return nil
	}
	if !isSteeringLabel(result.FollowupLabel) {
		return fmt.Errorf("%w %q", errUnsupportedSteeringLabel, result.FollowupLabel)
	}
	if result.FollowupLabel != "steering" {
		if hasSteeringFields(result) {
			return semanticTurnErrorMessage(errSemanticSteeringFieldsForOtherLabel, input.TurnID, "has steering-only fields for non-steering follow-up")
		}

		return nil
	}

	return validateSemanticSteeringRecord(input, result)
}

func validateSemanticSteeringRecord(input SemanticInput, result SemanticResult) error {
	if !isSteeringReason(result.SteeringReason) {
		return fmt.Errorf("%w %q", errUnsupportedSteeringReason, result.SteeringReason)
	}
	if err := validateSemanticPreventionRecord(input, result); err != nil {
		return err
	}

	return validateSemanticRecordConditionals(result)
}

func validateSemanticPreventionRecord(input SemanticInput, result SemanticResult) error {
	mechanismCount := len(result.PreventionMechanisms)
	if mechanismCount == 0 && !result.NotPreventable {
		return semanticTurnErrorMessage(errSemanticEmptyMechanismMustBeNotPreventable, input.TurnID, "has no prevention mechanism but is not marked not preventable")
	}
	if result.NotPreventable && mechanismCount != 0 {
		return fmt.Errorf("%w %q is marked not preventable but has applicable mechanisms", errSemanticNotPreventableHasMechanisms, input.TurnID)
	}
	if mechanismCount == 0 && result.PreventionConfidence != 0 {
		return semanticTurnErrorMessage(errSemanticPreventionConfidenceWithoutMechanism, input.TurnID, "has prevention confidence without an applicable prevention mechanism")
	}

	return validateSemanticMechanismValues(input, result)
}

func validateSemanticMechanismValues(input SemanticInput, result SemanticResult) error {
	allowed := map[string]struct{}{"agents_md": {}, "skill": {}, "task_prompt": {}, "validation": {}}
	used := make(map[string]struct{}, len(result.PreventionMechanisms))
	for _, mechanism := range result.PreventionMechanisms {
		if _, ok := allowed[mechanism]; !ok {
			return fmt.Errorf("%w %q", errUnsupportedPrevention, mechanism)
		}
		if _, duplicate := used[mechanism]; duplicate {
			return fmt.Errorf("%w %q has duplicate prevention mechanism %q", errSemanticDuplicateMechanism, input.TurnID, mechanism)
		}
		used[mechanism] = struct{}{}
	}

	return nil
}

func validateSemanticRecordConditionals(result SemanticResult) error {
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
		return fmt.Errorf("%w %q requires a valid follow-on classification, got %q", errSemanticConditionalRequiresValue, mechanism, value)
	}
	if !present && (value != "" || confidence != 0) {
		return fmt.Errorf("%w %q is present without prevention mechanism %q", errSemanticConditionalWithoutMechanism, value, mechanism)
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
Prefer 0-2 mechanisms per case; more than 2 is exceptional. If none clearly qualify, use prevention_mechanisms=[] and not_preventable=true. Otherwise use not_preventable=false. Do not mark a case not preventable when mechanisms are present. When prevention_mechanisms is non-empty, prevention_confidence must be a number; when it is empty, prevention_confidence must be null.

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

Every task with a non-empty prompt has task_type and task_confidence. If the task prompt is empty/unavailable, task_type and task_confidence must be null; do not use other as a substitute. A task with no follow-up must use empty/null follow-up fields. Follow-up classification remains fully required whenever a follow-up is supplied, including when the task prompt is unavailable. For follow-up labels other than steering, set steering_reason, steering_reason_confidence, prevention_confidence, prompt_issue, prompt_issue_confidence, agents_rule, agents_rule_confidence, skill_candidate, skill_confidence, validation_type, and validation_confidence to null; set prevention_mechanisms to [] and not_preventable to false. A steering follow-up must provide steering_reason and either one or more prevention mechanisms or not_preventable=true. Each optional follow-on classification and its confidence must be present exactly when its prevention mechanism applies. Return every supplied input ID exactly once, with no missing, duplicate, or unexpected IDs. Return no prose and do not use tools.`

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
