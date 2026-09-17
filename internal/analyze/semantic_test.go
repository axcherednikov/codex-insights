package analyze

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	analysiscache "codex-insights/internal/cache"
	"codex-insights/internal/sessions"
)

type semanticFakeRunner struct {
	mu                 sync.Mutex
	calls              int
	active             int
	maxActive          int
	failLarge          bool
	transientFailures  int
	invalidResponses   int
	requireEmptyTarget bool
	forcedError        error
	invalidTurnID      string
	failSingletonID    string
	lastPrompt         string
	delay              time.Duration
}

func (f *semanticFakeRunner) Run(prompt string, schema any, target any) error {
	f.mu.Lock()
	f.calls++
	f.lastPrompt = prompt
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	if f.transientFailures > 0 {
		f.mu.Lock()
		f.transientFailures--
		f.mu.Unlock()
		return errors.New("provider rate limit")
	}
	if f.forcedError != nil {
		return f.forcedError
	}
	records := strings.TrimSpace(prompt[strings.LastIndex(prompt, "Records:")+len("Records:"):])
	var ids []struct {
		TurnID string `json:"turn_id"`
		Prompt string `json:"prompt"`
	}
	if err := jsonUnmarshal([]byte(records), &ids); err != nil {
		return err
	}
	containsInvalid := false
	for _, item := range ids {
		if item.TurnID == f.invalidTurnID {
			containsInvalid = true
		}
	}
	if (f.failLarge && len(ids) > 2) || (containsInvalid && len(ids) > 1) {
		return nil
	}
	if len(ids) == 1 && ids[0].TurnID == f.failSingletonID {
		return errors.New("non-transient sibling failure")
	}
	response := target.(*semanticJudgeResponse)
	f.mu.Lock()
	targetHasResidualResults := f.requireEmptyTarget && len(response.Results) != 0
	invalidResponse := f.invalidResponses > 0
	if invalidResponse {
		f.invalidResponses--
	}
	f.mu.Unlock()
	if targetHasResidualResults {
		return errors.New("semantic runner received response target with residual results")
	}
	response.Results = make([]semanticJudgeResult, len(ids))
	for i, item := range ids {
		falseValue := false
		confidence := .9
		taskType := "research"
		result := semanticJudgeResult{TurnID: item.TurnID, TaskType: &taskType, TaskConfidence: &confidence, NotPreventable: &falseValue}
		if item.Prompt == "" {
			result.TaskType = nil
			result.TaskConfidence = nil
		}
		if invalidResponse {
			result.TaskType = nil
		}
		response.Results[i] = result
	}
	return nil
}

func TestSemanticEngineRejectsBlankExplicitFollowupBeforeRunner(t *testing.T) {
	runner := &semanticFakeRunner{}
	_, err := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 1, MaxRetries: 0}).Analyze([]SemanticInput{{
		TurnID:   "task",
		Prompt:   "task prompt",
		Followup: &SemanticFollowup{TurnID: "followup", Prompt: ""},
	}})
	if err == nil || !strings.Contains(err.Error(), "blank follow-up prompt") {
		t.Fatalf("Analyze error = %v, want blank follow-up validation error", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestSemanticEngineRetriesInvalidSingletonResponse(t *testing.T) {
	runner := &semanticFakeRunner{invalidResponses: 1, requireEmptyTarget: true}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 1, MaxRetries: 2, RetryBackoff: func(int) time.Duration { return 0 }})
	analysis, err := engine.Analyze(semanticInputs(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Results) != 1 || runner.calls != 2 {
		t.Fatalf("results=%#v calls=%d, want one result after two calls", analysis.Results, runner.calls)
	}
}

func TestSemanticEngineReturnsSemanticValidationErrorAfterInvalidSingletonRetries(t *testing.T) {
	runner := &semanticFakeRunner{invalidResponses: 3}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 1, MaxRetries: 2, RetryBackoff: func(int) time.Duration { return 0 }})
	_, err := engine.Analyze(semanticInputs(1))
	if err == nil || !strings.Contains(err.Error(), "requires task type and confidence") {
		t.Fatalf("Analyze error = %v, want semantic validation error", err)
	}
	if runner.calls != 3 {
		t.Fatalf("runner calls = %d, want 3", runner.calls)
	}
}

// Kept as a variable-sized helper to keep fake runner setup readable.
func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func semanticInputs(n int) []SemanticInput {
	result := make([]SemanticInput, n)
	for i := range result {
		result[i] = SemanticInput{TurnID: fmt.Sprintf("turn-%d", i), Prompt: fmt.Sprintf("prompt-%d", i)}
	}
	return result
}

func TestBuildSemanticInputsUsesOnlyExplicitFollowups(t *testing.T) {
	interactions := []sessions.Interaction{
		{TurnID: "session-a-tail", Prompt: "a"},
		{TurnID: "session-b-head", Prompt: "b"},
		{TurnID: "session-b-tail", Prompt: "c"},
		{TurnID: "empty-followup", Prompt: ""},
		{TurnID: "empty-omitted", Prompt: ""},
	}
	followups := []sessions.Followup{
		{PreviousTurnID: "session-b-head", TurnID: "session-b-tail", Prompt: "c", PreviousAnswer: "answer-b"},
		{PreviousTurnID: "empty-followup", TurnID: "next", Prompt: "continue", PreviousAnswer: "answer-empty"},
	}
	inputs := BuildSemanticInputs(interactions, followups)
	if len(inputs) != 4 {
		t.Fatalf("inputs = %d", len(inputs))
	}
	if inputs[0].Followup != nil {
		t.Fatal("cross-session tail was implicitly linked")
	}
	if inputs[1].Followup == nil || inputs[1].Followup.TurnID != "session-b-tail" {
		t.Fatalf("explicit follow-up not attached: %#v", inputs[1].Followup)
	}
	if inputs[2].Followup != nil {
		t.Fatal("follow-up target was incorrectly attached")
	}
	if inputs[3].TurnID != "empty-followup" || inputs[3].Followup == nil {
		t.Fatalf("empty-prompt explicit follow-up was dropped: %#v", inputs[3])
	}
}

func TestUnavailableTaskTypeIsNullableAndNotOther(t *testing.T) {
	input := SemanticInput{TurnID: "empty", Followup: &SemanticFollowup{TurnID: "next", Prompt: "continue"}}
	label, reason := "steering", "implementation_error"
	zero := float64(0)
	falseValue := true
	result := semanticJudgeResult{TurnID: "empty", TaskType: nil, TaskConfidence: nil, FollowupLabel: &label, FollowupConfidence: &zero, SteeringReason: &reason, SteeringReasonConfidence: &zero, NotPreventable: &falseValue}
	if err := validateSemanticJudgePresence(input, result); err != nil {
		t.Fatal(err)
	}
	converted := semanticResultFromJudge(result)
	if converted.TaskType != "" {
		t.Fatalf("unavailable task type became %q", converted.TaskType)
	}
	if _, ok := converted.ToTaskTypeResultIfAvailable(); ok {
		t.Fatal("unavailable task type produced a legacy result")
	}
	if err := ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{converted}); err != nil {
		t.Fatal(err)
	}
	input.Prompt = "available"
	if err := validateSemanticJudgePresence(input, result); err == nil {
		t.Fatal("null task type accepted for non-empty prompt")
	}
	taskType := "other"
	result.TaskType = &taskType
	if err := validateSemanticJudgePresence(input, result); err == nil {
		t.Fatal("null task confidence accepted for non-empty prompt")
	}
	result.TaskConfidence = &zero
	input.Prompt = ""
	if err := validateSemanticJudgePresence(input, result); err == nil {
		t.Fatal("fabricated task type accepted for unavailable prompt")
	}
}

func TestSemanticValidationRejectsMalformedAndInconsistentRecords(t *testing.T) {
	input := SemanticInput{TurnID: "one", Prompt: "one", Followup: &SemanticFollowup{TurnID: "two", Prompt: "next"}}
	valid := SemanticResult{TurnID: "one", TaskType: "feature", TaskConfidence: .8, FollowupLabel: "steering", SteeringReason: "implementation_error", PreventionMechanisms: []string{"validation"}, ValidationType: "tests"}
	cases := []struct {
		name   string
		result SemanticResult
	}{
		{"invalid enum", func() SemanticResult { r := valid; r.TaskType = "wat"; return r }()},
		{"duplicate mechanism", func() SemanticResult {
			r := valid
			r.PreventionMechanisms = []string{"validation", "validation"}
			return r
		}()},
		{"missing required follow-on", func() SemanticResult { r := valid; r.ValidationType = ""; return r }()},
		{"unexpected optional field", func() SemanticResult { r := valid; r.PromptIssue = "missing_context"; return r }()},
		{"not preventable mismatch", func() SemanticResult { r := valid; r.NotPreventable = true; return r }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{tc.result}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{valid, valid}); err == nil {
		t.Fatal("duplicate result accepted")
	}
	if err := ValidateSemanticResults([]SemanticInput{input}, []SemanticResult{{TurnID: "extra", TaskType: "feature"}}); err == nil {
		t.Fatal("unexpected result accepted")
	}
	if err := ValidateSemanticResults([]SemanticInput{input}, nil); err == nil {
		t.Fatal("missing result accepted")
	}

	noFollowup := SemanticInput{TurnID: "no-followup"}
	if err := ValidateSemanticResults([]SemanticInput{noFollowup}, []SemanticResult{{TurnID: "no-followup", TaskType: "feature", FollowupLabel: "continuation"}}); err == nil {
		t.Fatal("follow-up on task without follow-up accepted")
	}
}

func TestSemanticJudgeRequiresConfidenceWithApplicableLabels(t *testing.T) {
	input := SemanticInput{TurnID: "one", Prompt: "one", Followup: &SemanticFollowup{TurnID: "two", Prompt: "next"}}
	label, reason, validation := "steering", "implementation_error", "tests"
	taskConfidence := .9
	taskType := "feature"
	zero := float64(0)
	falseValue := false
	base := semanticJudgeResult{TurnID: "one", TaskType: &taskType, TaskConfidence: &taskConfidence, FollowupLabel: &label, FollowupConfidence: &zero, SteeringReason: &reason, SteeringReasonConfidence: &zero, PreventionMechanisms: []string{"validation"}, NotPreventable: &falseValue, PreventionConfidence: &zero, ValidationType: &validation, ValidationConfidence: &zero}
	if err := validateSemanticJudgePresence(input, base); err != nil {
		t.Fatal(err)
	}
	base.FollowupConfidence = nil
	if err := validateSemanticJudgePresence(input, base); err == nil {
		t.Fatal("missing follow-up confidence accepted")
	}
	base.FollowupConfidence = &zero
	base.SteeringReasonConfidence = nil
	if err := validateSemanticJudgePresence(input, base); err == nil {
		t.Fatal("missing steering reason confidence accepted")
	}
	base.SteeringReasonConfidence = &zero
	base.PreventionConfidence = nil
	if err := validateSemanticJudgePresence(input, base); err == nil {
		t.Fatal("missing prevention confidence accepted")
	}
	base.PreventionConfidence = &zero
	base.ValidationConfidence = nil
	if err := validateSemanticJudgePresence(input, base); err == nil {
		t.Fatal("missing conditional confidence accepted")
	}
}

func TestSemanticEngineSplitsRetriesAndPreservesInputOrder(t *testing.T) {
	runner := &semanticFakeRunner{failLarge: true}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 2, BatchSize: 5, MaxRetries: 0})
	inputs := semanticInputs(5)
	analysis, err := engine.Analyze(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Results) != len(inputs) {
		t.Fatalf("results = %d", len(analysis.Results))
	}
	for i, result := range analysis.Results {
		if result.TurnID != inputs[i].TurnID {
			t.Fatalf("result %d = %q", i, result.TurnID)
		}
	}
	if analysis.Stats.EvaluatedRecords != 5 || analysis.Stats.EvaluatedBatches != 3 {
		t.Fatalf("stats = %+v", analysis.Stats)
	}
	if runner.maxActive > 2 {
		t.Fatalf("max concurrent calls = %d, want <= 2", runner.maxActive)
	}
	if runner.calls != 5 {
		t.Fatalf("calls = %d, want initial plus 2 children and their singleton split calls", runner.calls)
	}
}

func TestSemanticEngineUsesBoundedConcurrentWorkers(t *testing.T) {
	runner := &semanticFakeRunner{delay: 5 * time.Millisecond}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 2, BatchSize: 2, MaxRetries: 0})
	if _, err := engine.Analyze(semanticInputs(8)); err != nil {
		t.Fatal(err)
	}
	if runner.maxActive <= 1 {
		t.Fatalf("workers did not overlap: max active = %d", runner.maxActive)
	}
	if runner.maxActive > 2 {
		t.Fatalf("worker cap exceeded: max active = %d", runner.maxActive)
	}
}

func TestSemanticProgressReportsCacheLookupAndPersistedChildrenSerially(t *testing.T) {
	store, err := analysiscache.New(t.TempDir() + "/cache.json")
	if err != nil {
		t.Fatal(err)
	}
	runner := &semanticFakeRunner{failLarge: true}
	var mu sync.Mutex
	var events []SemanticProgress
	active := 0
	maxActive := 0
	config := SemanticConfig{Runner: runner, Cache: store, Workers: 1, BatchSize: 3, MaxRetries: 0, Progress: func(progress SemanticProgress) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		events = append(events, progress)
		time.Sleep(time.Millisecond)
		active--
		mu.Unlock()
	}}
	inputs := semanticInputs(3)
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want initial plus 2 child batches", len(events))
	}
	if events[0].CompletedRecords != 0 || events[1].CompletedRecords != 1 || events[2].CompletedRecords != 3 {
		t.Fatalf("progress = %+v", events)
	}
	if maxActive != 1 {
		t.Fatalf("progress callback concurrency = %d", maxActive)
	}
	events = nil
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].CacheHits != 3 || events[0].CompletedRecords != 3 {
		t.Fatalf("cache progress = %+v", events)
	}
}

func TestSemanticEngineRetriesOnlyTransientFailures(t *testing.T) {
	runner := &semanticFakeRunner{transientFailures: 1}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 2, MaxRetries: 2, RetryBackoff: func(int) time.Duration { return 0 }})
	analysis, err := engine.Analyze(semanticInputs(2))
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 2 || analysis.Stats.EvaluatedRecords != 2 {
		t.Fatalf("calls=%d stats=%+v", runner.calls, analysis.Stats)
	}
}

func TestSemanticEngineKeepsSuccessfulChildCacheWhenSiblingFails(t *testing.T) {
	store, err := analysiscache.New(t.TempDir() + "/cache.json")
	if err != nil {
		t.Fatal(err)
	}
	inputs := semanticInputs(4)
	runner := &semanticFakeRunner{failLarge: true, invalidTurnID: inputs[3].TurnID, failSingletonID: inputs[3].TurnID}
	// The fake returns a valid left child, then forces the rightmost singleton
	// to fail after its parent has been split.
	runnerSingleton := runner
	config := SemanticConfig{Runner: runnerSingleton, Cache: store, Workers: 1, BatchSize: 4, MaxRetries: 0}
	if _, err := NewSemanticEngine(config).Analyze(inputs); err == nil {
		t.Fatal("expected later sibling failure")
	}
	// The first two records were a validated child batch and must survive.
	for _, input := range inputs[:2] {
		var cached SemanticResult
		if !store.Get(SemanticCacheKey(input, NewSemanticEngine(config).config), &cached) {
			t.Fatalf("child result for %s was not persisted", input.TurnID)
		}
	}
	firstCalls := runner.calls
	if _, err := NewSemanticEngine(config).Analyze(inputs); err == nil {
		t.Fatal("expected repeated failing sibling")
	}
	if runner.calls != firstCalls+1 {
		t.Fatalf("rerun did not reuse successful children: calls %d -> %d", firstCalls, runner.calls)
	}
}

func TestSemanticRunnerFailuresAreNotSplit(t *testing.T) {
	inputs := semanticInputs(4)
	runner := &semanticFakeRunner{forcedError: errors.New("permission denied")}
	engine := NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 4, MaxRetries: 3, RetryBackoff: func(int) time.Duration { return 0 }})
	if _, err := engine.Analyze(inputs); err == nil {
		t.Fatal("non-transient failure unexpectedly succeeded")
	}
	if runner.calls != 1 {
		t.Fatalf("non-transient calls = %d, want 1", runner.calls)
	}

	runner = &semanticFakeRunner{transientFailures: 20}
	engine = NewSemanticEngine(SemanticConfig{Runner: runner, Workers: 1, BatchSize: 4, MaxRetries: 2, RetryBackoff: func(int) time.Duration { return 0 }})
	if _, err := engine.Analyze(inputs); err == nil {
		t.Fatal("exhausted transient failure unexpectedly succeeded")
	}
	if runner.calls != 3 {
		t.Fatalf("exhausted transient calls = %d, want 3 without splitting", runner.calls)
	}
}

func TestSemanticEngineCacheIsLanguageIndependentAndInvalidatesVersions(t *testing.T) {
	path := t.TempDir() + "/cache.json"
	store, err := analysiscache.New(path)
	if err != nil {
		t.Fatal(err)
	}
	runner := &semanticFakeRunner{}
	config := SemanticConfig{Runner: runner, Cache: store, Workers: 1, BatchSize: 2, MaxRetries: 0}
	inputs := []SemanticInput{{TurnID: "one", Prompt: "private conversation text"}}
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private conversation text") {
		t.Fatal("cache stored raw input")
	}
	if strings.Contains(runner.lastPrompt, `"answer"`) {
		t.Fatal("task answer was serialized without a follow-up")
	}
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("cache miss on repeat, calls=%d", runner.calls)
	}
	config.PromptVersion = "semantic-judge-v2"
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 2 {
		t.Fatalf("prompt version did not invalidate cache, calls=%d", runner.calls)
	}
	config.MethodologyVersion = "semantic-v2"
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 3 {
		t.Fatalf("methodology version did not invalidate cache, calls=%d", runner.calls)
	}
	config.SchemaVersion = "semantic-schema-test-next"
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 4 {
		t.Fatalf("schema version did not invalidate cache, calls=%d", runner.calls)
	}
	config.Model = "different-model"
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 5 {
		t.Fatalf("model did not invalidate cache, calls=%d", runner.calls)
	}
	inputs[0].Prompt = "changed task prompt"
	if _, err := NewSemanticEngine(config).Analyze(inputs); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 6 {
		t.Fatalf("input content did not invalidate cache, calls=%d", runner.calls)
	}
}

func TestSemanticSchemaIsStrictAndEnumerated(t *testing.T) {
	schema := semanticSchema()
	if schema["additionalProperties"] != false {
		t.Fatal("root schema is not strict")
	}
	properties := schema["properties"].(map[string]any)
	results := properties["results"].(map[string]any)
	item := results["items"].(map[string]any)
	if item["additionalProperties"] != false {
		t.Fatal("record schema is not strict")
	}
	field := item["properties"].(map[string]any)["followup_label"].(map[string]any)
	if _, ok := field["anyOf"]; !ok {
		t.Fatal("nullable enum schema missing")
	}
}

func TestSemanticJudgePromptRetainsClassifierMethodology(t *testing.T) {
	for _, required := range []string{
		"architecture = system design, architecture decisions, or decomposition",
		"research = investigation, explanation, comparison, or technical learning",
		"steering = user corrects or redirects Codex because previous work or understanding was wrong",
		"user_correction = user changes or corrects their own requirement, not a Codex mistake",
		"Prefer 0-2 mechanisms per case; more than 2 is exceptional",
		"not_preventable=true",
		"task_prompt = missing or ambiguous information in THIS request materially caused the mistake",
		"Use only rules that could be written as a recurring project instruction.",
		"Generalize only workflows useful across multiple projects; use other for highly specific cases.",
		"Return every supplied input ID exactly once, with no missing, duplicate, or unexpected IDs.",
		"Return no prose and do not use tools.",
	} {
		if !strings.Contains(semanticJudgePrompt, required) {
			t.Fatalf("semantic Judge prompt missing methodology text %q", required)
		}
	}
}
