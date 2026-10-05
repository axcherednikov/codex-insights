// Package golden contains the format, privacy-preserving export, evaluation
// metrics, and historical regression guard for semantic Judge evaluations.
package golden

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/fileio"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

const (
	FormatVersion         = "golden-v1"
	ApprovalCandidate     = "candidate"
	ApprovalApproved      = "approved"
	MaxTextLength         = 2400
	textTruncationDivisor = 2
)

type Methodology struct {
	MethodologyVersion string `json:"methodology_version"`
	PromptVersion      string `json:"prompt_version"`
	SchemaVersion      string `json:"schema_version"`
}

type Provenance struct {
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
	Source     string `json:"source,omitempty"`
}

// Expected uses omitted fields for partially labelled cases. Prevention
// mechanisms intentionally use a nil pointer versus a pointer to an empty
// slice to retain the distinction between unlabelled and an approved empty
// set.
type Expected struct {
	TaskType             string    `json:"task_type,omitempty"`
	FollowupLabel        string    `json:"followup_label,omitempty"`
	SteeringReason       string    `json:"steering_reason,omitempty"`
	PromptIssue          string    `json:"prompt_issue,omitempty"`
	AgentsRule           string    `json:"agents_rule,omitempty"`
	SkillCandidate       string    `json:"skill_candidate,omitempty"`
	ValidationType       string    `json:"validation_type,omitempty"`
	PreventionMechanisms *[]string `json:"prevention_mechanisms,omitempty"`
}

type Case struct {
	ID             string    `json:"id"`
	Prompt         string    `json:"prompt,omitempty"`
	PreviousAnswer string    `json:"previous_answer,omitempty"`
	FollowupPrompt string    `json:"followup_prompt,omitempty"`
	Expected       *Expected `json:"expected,omitempty"`
}

type Fixture struct {
	FormatVersion string      `json:"format_version"`
	Methodology   Methodology `json:"methodology"`
	Provenance    Provenance  `json:"provenance"`
	Cases         []Case      `json:"cases"`
}

func (f Fixture) ValidateForEvaluation() error {
	if err := f.validateMetadata(); err != nil {
		return err
	}

	return f.validateCases()
}

func (f Fixture) validateMetadata() error {
	if f.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported golden format version %q: %w", f.FormatVersion, errUnsupportedFormatVersion)
	}
	if f.Methodology.MethodologyVersion == "" || f.Methodology.PromptVersion == "" || f.Methodology.SchemaVersion == "" {
		return errIncompleteMethodology
	}
	if f.Provenance.Status != ApprovalApproved || strings.TrimSpace(f.Provenance.ApprovedBy) == "" || strings.TrimSpace(f.Provenance.ApprovedAt) == "" || strings.TrimSpace(f.Provenance.Source) == "" {
		return errIncompleteApproval
	}
	if _, err := time.Parse(time.RFC3339, f.Provenance.ApprovedAt); err != nil {
		return fmt.Errorf("invalid approval timestamp: %w: %w", errApprovalTimestamp, err)
	}
	if len(f.Cases) == 0 {
		return errEmptyFixture
	}

	return nil
}

func (f Fixture) validateCases() error {
	seen := make(map[string]struct{}, len(f.Cases))
	for _, c := range f.Cases {
		if err := validateCase(c, seen); err != nil {
			return err
		}
	}

	return nil
}

func validateCase(c Case, seen map[string]struct{}) error {
	if !validCaseID(c.ID) {
		return fmt.Errorf("invalid golden case id %q: %w", c.ID, errInvalidCaseID)
	}
	if _, ok := seen[c.ID]; ok {
		return fmt.Errorf("duplicate golden case id %q: %w", c.ID, errDuplicateCaseID)
	}
	seen[c.ID] = struct{}{}
	if c.Prompt == "" && c.FollowupPrompt == "" {
		return fmt.Errorf("golden case %q has no semantic input: %w", c.ID, errCaseWithoutInput)
	}
	if c.Expected == nil {
		return fmt.Errorf("golden case %q has no expected semantic fields: %w", c.ID, errCaseWithoutExpected)
	}
	if !expectedHasFields(*c.Expected) {
		return fmt.Errorf("golden case %q has an empty expected object: %w", c.ID, errEmptyExpected)
	}
	if err := validateExpected(*c.Expected, c.Prompt != "", c.FollowupPrompt != ""); err != nil {
		return fmt.Errorf("golden case %q: %w", c.ID, err)
	}

	return nil
}

func expectedHasFields(e Expected) bool {
	return e.TaskType != "" || e.FollowupLabel != "" || e.SteeringReason != "" || e.PromptIssue != "" || e.AgentsRule != "" || e.SkillCandidate != "" || e.ValidationType != "" || e.PreventionMechanisms != nil
}

func validCaseID(id string) bool {
	if !strings.HasPrefix(id, "case-") || len(id) < 7 {
		return false
	}
	suffix := id[5:]
	for _, r := range suffix {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}

	return true
}

func validTaskType(value string) bool {
	switch value {
	case "bugfix", "feature", "refactor", "tests", "code_review", "architecture", "devops", "research", "documentation", "other":
		return true
	default:
		return false
	}
}

func validFollowupLabel(value string) bool {
	switch value {
	case "steering", "continuation", "user_correction", "question":
		return true
	default:
		return false
	}
}

func validSteeringReason(value string) bool {
	switch value {
	case "misunderstood_request", "overengineering", "implementation_error", "ignored_constraints", "insufficient_validation", "architecture_mismatch", "wrong_output_format", "other":
		return true
	default:
		return false
	}
}

func validPromptIssue(value string) bool {
	switch value {
	case "missing_constraints", "missing_context", "ambiguous_request", "missing_acceptance", "missing_scope", "wrong_assumption", "other":
		return true
	default:
		return false
	}
}

func validAgentsRule(value string) bool {
	switch value {
	case "avoid_unnecessary_complexity", "preserve_project_architecture", "follow_explicit_constraints", "avoid_scope_creep", "inspect_existing_code_first", "validate_before_completion", "follow_requested_output_format", "other":
		return true
	default:
		return false
	}
}

func validSkillCandidate(value string) bool {
	switch value {
	case "implementation_prompt_file", "bounded_architecture_review", "verification_workflow", "other":
		return true
	default:
		return false
	}
}

func validValidationType(value string) bool {
	switch value {
	case "runtime_smoke_check", "deployment_check", "diff_review", "static_analysis", "output_validation", "tests", "data_validation", "other":
		return true
	default:
		return false
	}
}

func validMechanism(value string) bool {
	switch value {
	case "agents_md", "skill", "task_prompt", "validation":
		return true
	default:
		return false
	}
}

func validateExpected(e Expected, hasPrompt, hasFollowup bool) error {
	if err := validateExpectedTaxonomies(e); err != nil {
		return err
	}
	if err := validateExpectedApplicability(e, hasPrompt, hasFollowup); err != nil {
		return err
	}
	if err := validateExpectedMechanisms(e); err != nil {
		return err
	}

	return validateExpectedSteering(e)
}

func validateExpectedTaxonomies(e Expected) error {
	checks := []struct {
		value string
		valid func(string) bool
		err   error
	}{
		{e.TaskType, validTaskType, errInvalidTaskType},
		{e.FollowupLabel, validFollowupLabel, errInvalidFollowupLabel},
		{e.SteeringReason, validSteeringReason, errInvalidSteeringReason},
		{e.PromptIssue, validPromptIssue, errInvalidPromptIssue},
		{e.AgentsRule, validAgentsRule, errInvalidAgentsRule},
		{e.SkillCandidate, validSkillCandidate, errInvalidSkillCandidate},
		{e.ValidationType, validValidationType, errInvalidValidationType},
	}
	for _, check := range checks {
		if check.value != "" && !check.valid(check.value) {
			return fmt.Errorf("%s %q: %w", check.err.Error(), check.value, check.err)
		}
	}

	return nil
}

func validateExpectedApplicability(e Expected, hasPrompt, hasFollowup bool) error {
	if e.TaskType != "" && !hasPrompt {
		return errTaskTypeNeedsPrompt
	}
	if !hasFollowup && hasFollowupExpectedFields(e) {
		return errFollowupNeedsPrompt
	}

	return nil
}

func hasFollowupExpectedFields(e Expected) bool {
	return e.FollowupLabel != "" || e.SteeringReason != "" || e.PromptIssue != "" || e.AgentsRule != "" || e.SkillCandidate != "" || e.ValidationType != "" || e.PreventionMechanisms != nil
}

func validateExpectedMechanisms(e Expected) error {
	var prevention []string
	if e.PreventionMechanisms != nil {
		prevention = *e.PreventionMechanisms
	}

	return validateMechanismValues(prevention)
}

func validateMechanismValues(prevention []string) error {
	seen := make(map[string]struct{}, len(prevention))
	for _, m := range prevention {
		if !validMechanism(m) {
			return fmt.Errorf("invalid prevention mechanism %q: %w", m, errInvalidMechanism)
		}
		if _, duplicate := seen[m]; duplicate {
			return fmt.Errorf("duplicate prevention mechanism %q: %w", m, errDuplicateMechanism)
		}
		seen[m] = struct{}{}
	}

	return nil
}

func validateExpectedSteering(e Expected) error {
	if e.SteeringReason != "" && e.FollowupLabel != "" && e.FollowupLabel != "steering" {
		return errSteeringReasonNeedsLabel
	}
	if e.FollowupLabel != "" && e.FollowupLabel != "steering" && hasSteeringOnlyFields(e) {
		return errSteeringOnlyNeedsLabel
	}
	if e.PreventionMechanisms == nil {
		return nil
	}

	return validateConditionalMechanisms(e, *e.PreventionMechanisms)
}

func hasSteeringOnlyFields(e Expected) bool {
	return e.PromptIssue != "" || e.AgentsRule != "" || e.SkillCandidate != "" || e.ValidationType != "" || e.PreventionMechanisms != nil && len(*e.PreventionMechanisms) != 0
}

func validateConditionalMechanisms(e Expected, mechanisms []string) error {
	conditions := []struct {
		name, mechanism string
		present         bool
	}{
		{"task_prompt", "task_prompt", e.PromptIssue != ""},
		{"agents_md", "agents_md", e.AgentsRule != ""},
		{"skill", "skill", e.SkillCandidate != ""},
		{"validation", "validation", e.ValidationType != ""},
	}
	for _, condition := range conditions {
		if condition.present && !contains(mechanisms, condition.mechanism) {
			return fmt.Errorf("%s expected field requires matching prevention mechanism: %w", condition.name, errMechanismRequired)
		}
	}

	return nil
}

func Read(path string) (Fixture, error) {
	data, err := fileio.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("read golden fixture: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var fixture Fixture
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode golden fixture: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Fixture{}, errTrailingJSON
		}

		return Fixture{}, fmt.Errorf("decode trailing golden fixture data: %w", err)
	}

	return fixture, nil
}

func Write(path string, fixture Fixture) error {
	if strings.TrimSpace(path) == "" {
		return errOutputPathRequired
	}
	if fixture.FormatVersion == "" {
		fixture.FormatVersion = FormatVersion
	}
	// Fixture uses only strings, concrete structs, and concrete slices/pointers;
	// no field implements json.Marshaler, so this encoding cannot fail.
	data, _ := json.MarshalIndent(fixture, "", "  ")

	if err := fileio.WriteAtomic(path, ".golden-", append(data, '\n')); err != nil {
		return fmt.Errorf("write golden fixture: %w", err)
	}

	return nil
}

type ExportInput struct {
	ID             string
	Prompt         string
	PreviousAnswer string
	FollowupPrompt string
}

func BuildCandidate(inputs []ExportInput, limit int, method Methodology) Fixture {
	if limit <= 0 || limit > len(inputs) {
		limit = len(inputs)
	}
	items := append([]ExportInput(nil), inputs...)
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.ToLower(items[i].Prompt + "\x00" + items[i].FollowupPrompt)
		right := strings.ToLower(items[j].Prompt + "\x00" + items[j].FollowupPrompt)
		if left != right {
			return left < right
		}

		return items[i].ID < items[j].ID
	})
	cases := make([]Case, 0, limit)
	for i, item := range items[:limit] {
		cases = append(cases, Case{ID: fmt.Sprintf("case-%06d", i+1), Prompt: Redact(item.Prompt), PreviousAnswer: Redact(item.PreviousAnswer), FollowupPrompt: Redact(item.FollowupPrompt)})
	}

	return Fixture{FormatVersion: FormatVersion, Methodology: method, Provenance: Provenance{Status: ApprovalCandidate}, Cases: cases}
}

var (
	emailRE = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	tokenRE = regexp.MustCompile(`(?i)\b(?:bearer\s+|token\s*[=:]\s*|api[_-]?key\s*[=:]\s*|sk-[a-z0-9_-]+|gh[pousr]_[a-z0-9_-]+)[^\s,;]*`)
	homeRE  = regexp.MustCompile(`/Users/[^\s"']+|/home/[^\s"']+|[A-Za-z]:\\Users\\[^\s"']+`)
	fenceRE = regexp.MustCompile("(?s)```.*?```")
)

func Redact(text string) string {
	text = fenceRE.ReplaceAllString(text, "[redacted code]")
	text = emailRE.ReplaceAllString(text, "[redacted email]")
	text = tokenRE.ReplaceAllString(text, "[redacted token]")
	text = homeRE.ReplaceAllString(text, "[redacted local path]")

	return capText(text, MaxTextLength)
}

func capText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	head := limit / textTruncationDivisor
	tail := limit - head

	return string(runes[:head]) + "\n...[truncated]...\n" + string(runes[len(runes)-tail:])
}

func Inputs(f Fixture) []analyze.SemanticInput {
	inputs := make([]analyze.SemanticInput, 0, len(f.Cases))
	for _, c := range f.Cases {
		input := analyze.SemanticInput{TurnID: c.ID, Prompt: c.Prompt}
		if c.FollowupPrompt != "" {
			input.Followup = &analyze.SemanticFollowup{TurnID: c.ID + "-followup", Prompt: c.FollowupPrompt, PreviousAnswer: c.PreviousAnswer}
		}
		inputs = append(inputs, input)
	}

	return inputs
}

type FieldMetrics struct {
	Samples  int     `json:"samples"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}
type Confusion struct {
	Field     string `json:"field"`
	Expected  string `json:"expected"`
	Predicted string `json:"predicted"`
	Count     int    `json:"count"`
}
type PreventionMetrics struct {
	Samples      int     `json:"samples"`
	ExactMatches int     `json:"exact_matches"`
	Precision    float64 `json:"precision"`
	Recall       float64 `json:"recall"`
	F1           float64 `json:"f1"`
}
type Metrics struct {
	Fields     map[string]FieldMetrics `json:"fields"`
	Prevention PreventionMetrics       `json:"prevention"`
	Confusions []Confusion             `json:"confusions"`
}

// Evaluate runs the current unified semantic engine against an approved
// fixture. The cache is deliberately nil so this path always exercises the
// supplied Judge runner; the progress callback is used by the CLI for its
// pre-Judge disclosure and cold-run progress.
func Evaluate(fixture Fixture, runner analyze.SemanticRunner, workers int, progress func(analyze.SemanticProgress)) (analyze.SemanticAnalysis, Metrics, error) {
	if err := fixture.ValidateForEvaluation(); err != nil {
		return analyze.SemanticAnalysis{}, Metrics{}, fmt.Errorf("validate fixture for evaluation: %w", err)
	}
	if workers < 1 || workers > 32 {
		return analyze.SemanticAnalysis{}, Metrics{}, fmt.Errorf("invalid evaluation concurrency %d: %w", workers, errInvalidConcurrency)
	}
	config := analyze.DefaultSemanticConfig()
	config.Runner = runner
	config.Cache = nil
	config.Workers = workers
	config.Progress = progress
	analysisResult, err := analyze.NewSemanticEngine(config).Analyze(Inputs(fixture))
	if err != nil {
		return analyze.SemanticAnalysis{}, Metrics{}, fmt.Errorf("evaluate semantic fixture: %w", err)
	}
	metrics, err := EvaluateMetrics(fixture, analysisResult.Results)
	if err != nil {
		return analyze.SemanticAnalysis{}, Metrics{}, err
	}

	return analysisResult, metrics, nil
}

func EvaluateMetrics(f Fixture, results []analyze.SemanticResult) (Metrics, error) {
	byID, err := indexMetricResults(f, results)
	if err != nil {
		return Metrics{}, err
	}
	metrics := Metrics{Fields: make(map[string]FieldMetrics)}
	confusionCounts := make(map[string]int)
	preventionTotals := preventionMetricTotals{}
	for _, c := range f.Cases {
		if c.Expected == nil {
			continue
		}
		predicted := byID[c.ID]
		accumulateScalarMetrics(&metrics, confusionCounts, *c.Expected, predicted)
		accumulatePreventionMetrics(&metrics, confusionCounts, &preventionTotals, *c.Expected, predicted)
	}
	finalizeScalarMetrics(&metrics)
	finalizePreventionMetrics(&metrics, preventionTotals)
	metrics.Confusions = confusionResults(confusionCounts)

	return metrics, nil
}

func indexMetricResults(f Fixture, results []analyze.SemanticResult) (map[string]analyze.SemanticResult, error) {
	if len(f.Cases) != len(results) {
		return nil, fmt.Errorf("metrics received %d results for %d cases: %w", len(results), len(f.Cases), errMetricsResultCount)
	}
	byID := make(map[string]analyze.SemanticResult, len(results))
	for _, result := range results {
		if _, duplicate := byID[result.TurnID]; duplicate {
			return nil, fmt.Errorf("metrics received duplicate result id %q: %w", result.TurnID, errMetricsDuplicateResult)
		}
		byID[result.TurnID] = result
	}
	for _, item := range f.Cases {
		if _, ok := byID[item.ID]; !ok {
			return nil, fmt.Errorf("metrics missing result for case %q: %w", item.ID, errMetricsMissingResult)
		}
	}

	return byID, nil
}

func accumulateScalarMetrics(metrics *Metrics, confusionCounts map[string]int, expected Expected, predicted analyze.SemanticResult) {
	fields := []struct{ name, expected, predicted string }{
		{"task_type", expected.TaskType, predicted.TaskType},
		{"followup_label", expected.FollowupLabel, predicted.FollowupLabel},
		{"steering_reason", expected.SteeringReason, predicted.SteeringReason},
		{"prompt_issue", expected.PromptIssue, predicted.PromptIssue},
		{"agents_rule", expected.AgentsRule, predicted.AgentsRule},
		{"skill_candidate", expected.SkillCandidate, predicted.SkillCandidate},
		{"validation_type", expected.ValidationType, predicted.ValidationType},
	}
	for _, field := range fields {
		accumulateScalarField(metrics.Fields, confusionCounts, field.name, field.expected, field.predicted)
	}
}

func accumulateScalarField(fields map[string]FieldMetrics, confusionCounts map[string]int, name, expected, predicted string) {
	if expected == "" {
		return
	}
	stat := fields[name]
	stat.Samples++
	if expected == predicted {
		stat.Correct++
	} else {
		confusionCounts[name+"\x00"+expected+"\x00"+predicted]++
	}
	fields[name] = stat
}

type preventionMetricTotals struct {
	predicted int
	expected  int
}

func accumulatePreventionMetrics(metrics *Metrics, confusionCounts map[string]int, totals *preventionMetricTotals, expected Expected, predicted analyze.SemanticResult) {
	if expected.PreventionMechanisms == nil {
		return
	}
	metrics.Prevention.Samples++
	want := unique(*expected.PreventionMechanisms)
	got := unique(predicted.PreventionMechanisms)
	if equalStrings(want, got) {
		metrics.Prevention.ExactMatches++
	} else {
		confusionCounts["prevention\x00"+strings.Join(want, ",")+"\x00"+strings.Join(got, ",")]++
	}
	for _, value := range got {
		if contains(want, value) {
			metrics.Prevention.Precision++
		}
	}
	for _, value := range want {
		if contains(got, value) {
			metrics.Prevention.Recall++
		}
	}
	totals.predicted += len(got)
	totals.expected += len(want)
}

func finalizeScalarMetrics(metrics *Metrics) {
	for name, stat := range metrics.Fields {
		if stat.Samples > 0 {
			stat.Accuracy = float64(stat.Correct) / float64(stat.Samples)
			metrics.Fields[name] = stat
		}
	}
}

func finalizePreventionMetrics(metrics *Metrics, totals preventionMetricTotals) {
	if metrics.Prevention.Samples == 0 {
		return
	}
	if totals.predicted > 0 {
		metrics.Prevention.Precision /= float64(totals.predicted)
	}
	if totals.expected > 0 {
		metrics.Prevention.Recall /= float64(totals.expected)
	}
	precisionRecall := metrics.Prevention.Precision + metrics.Prevention.Recall
	if precisionRecall > 0 {
		metrics.Prevention.F1 = 2 * metrics.Prevention.Precision * metrics.Prevention.Recall / precisionRecall
	}
}

func confusionResults(confusionCounts map[string]int) []Confusion {
	confusions := make([]Confusion, 0, len(confusionCounts))
	for key, count := range confusionCounts {
		parts := strings.Split(key, "\x00")
		confusions = append(confusions, Confusion{Field: parts[0], Expected: parts[1], Predicted: parts[2], Count: count})
	}
	sort.Slice(confusions, func(i, j int) bool {
		a, b := confusions[i], confusions[j]
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Expected != b.Expected {
			return a.Expected < b.Expected
		}

		return a.Predicted < b.Predicted
	})

	return confusions
}

func unique(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	j := 0
	for _, v := range out {
		if j == 0 || out[j-1] != v {
			out[j] = v
			j++
		}
	}

	return out[:j]
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// CollectCandidateInputs converts already-windowed local interactions to the
// minimized export shape. It is intentionally Judge-free.
func CollectCandidateInputs(interactions []sessions.Interaction, followups []sessions.Followup) []ExportInput {
	byPrevious := make(map[string]sessions.Followup, len(followups))
	for _, f := range followups {
		byPrevious[f.PreviousTurnID] = f
	}
	items := make([]ExportInput, 0, len(interactions))
	for _, item := range interactions {
		f, ok := byPrevious[item.TurnID]
		if !ok && item.Prompt == "" {
			continue
		}
		if !ok {
			items = append(items, ExportInput{ID: item.TurnID, Prompt: item.Prompt})

			continue
		}
		items = append(items, ExportInput{ID: item.TurnID, Prompt: item.Prompt, PreviousAnswer: f.PreviousAnswer, FollowupPrompt: f.Prompt})
	}

	return items
}
