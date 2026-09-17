// Package golden contains the format, privacy-preserving export, evaluation
// metrics, and historical regression guard for semantic Judge evaluations.
package golden

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

const (
	FormatVersion     = "golden-v1"
	ApprovalCandidate = "candidate"
	ApprovalApproved  = "approved"
	MaxTextLength     = 2400
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
	if f.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported golden format version %q", f.FormatVersion)
	}
	if f.Methodology.MethodologyVersion == "" || f.Methodology.PromptVersion == "" || f.Methodology.SchemaVersion == "" {
		return errors.New("golden methodology metadata is incomplete")
	}
	if f.Provenance.Status != ApprovalApproved || strings.TrimSpace(f.Provenance.ApprovedBy) == "" || strings.TrimSpace(f.Provenance.ApprovedAt) == "" || strings.TrimSpace(f.Provenance.Source) == "" {
		return errors.New("golden fixture is not approved with complete provenance")
	}
	if _, err := time.Parse(time.RFC3339, f.Provenance.ApprovedAt); err != nil {
		return fmt.Errorf("invalid approval timestamp: %w", err)
	}
	if len(f.Cases) == 0 {
		return errors.New("golden fixture has no cases")
	}
	seen := make(map[string]struct{}, len(f.Cases))
	for _, c := range f.Cases {
		if !validCaseID(c.ID) {
			return fmt.Errorf("invalid golden case id %q", c.ID)
		}
		if _, ok := seen[c.ID]; ok {
			return fmt.Errorf("duplicate golden case id %q", c.ID)
		}
		seen[c.ID] = struct{}{}
		if c.Prompt == "" && c.FollowupPrompt == "" {
			return fmt.Errorf("golden case %q has no semantic input", c.ID)
		}
		if c.Expected == nil {
			return fmt.Errorf("golden case %q has no expected semantic fields", c.ID)
		}
		if !expectedHasFields(*c.Expected) {
			return fmt.Errorf("golden case %q has an empty expected object", c.ID)
		}
		if err := validateExpected(*c.Expected, c.Prompt != "", c.FollowupPrompt != ""); err != nil {
			return fmt.Errorf("golden case %q: %w", c.ID, err)
		}
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

var (
	taskTypes       = map[string]bool{"bugfix": true, "feature": true, "refactor": true, "tests": true, "code_review": true, "architecture": true, "devops": true, "research": true, "documentation": true, "other": true}
	followupLabels  = map[string]bool{"steering": true, "continuation": true, "user_correction": true, "question": true}
	steeringReasons = map[string]bool{"misunderstood_request": true, "overengineering": true, "implementation_error": true, "ignored_constraints": true, "insufficient_validation": true, "architecture_mismatch": true, "wrong_output_format": true, "other": true}
	promptIssues    = map[string]bool{"missing_constraints": true, "missing_context": true, "ambiguous_request": true, "missing_acceptance": true, "missing_scope": true, "wrong_assumption": true, "other": true}
	agentsRules     = map[string]bool{"avoid_unnecessary_complexity": true, "preserve_project_architecture": true, "follow_explicit_constraints": true, "avoid_scope_creep": true, "inspect_existing_code_first": true, "validate_before_completion": true, "follow_requested_output_format": true, "other": true}
	skillCandidates = map[string]bool{"implementation_prompt_file": true, "bounded_architecture_review": true, "verification_workflow": true, "other": true}
	validationTypes = map[string]bool{"runtime_smoke_check": true, "deployment_check": true, "diff_review": true, "static_analysis": true, "output_validation": true, "tests": true, "data_validation": true, "other": true}
	mechanisms      = map[string]bool{"agents_md": true, "skill": true, "task_prompt": true, "validation": true}
)

func validateExpected(e Expected, hasPrompt, hasFollowup bool) error {
	if e.TaskType != "" && !taskTypes[e.TaskType] {
		return fmt.Errorf("invalid task_type %q", e.TaskType)
	}
	if e.FollowupLabel != "" && !followupLabels[e.FollowupLabel] {
		return fmt.Errorf("invalid followup_label %q", e.FollowupLabel)
	}
	if e.SteeringReason != "" && !steeringReasons[e.SteeringReason] {
		return fmt.Errorf("invalid steering_reason %q", e.SteeringReason)
	}
	if e.PromptIssue != "" && !promptIssues[e.PromptIssue] {
		return fmt.Errorf("invalid prompt_issue %q", e.PromptIssue)
	}
	if e.AgentsRule != "" && !agentsRules[e.AgentsRule] {
		return fmt.Errorf("invalid agents_rule %q", e.AgentsRule)
	}
	if e.SkillCandidate != "" && !skillCandidates[e.SkillCandidate] {
		return fmt.Errorf("invalid skill_candidate %q", e.SkillCandidate)
	}
	if e.ValidationType != "" && !validationTypes[e.ValidationType] {
		return fmt.Errorf("invalid validation_type %q", e.ValidationType)
	}
	if e.TaskType != "" && !hasPrompt {
		return errors.New("task_type requires a prompt")
	}
	if !hasFollowup && (e.FollowupLabel != "" || e.SteeringReason != "" || e.PromptIssue != "" || e.AgentsRule != "" || e.SkillCandidate != "" || e.ValidationType != "" || e.PreventionMechanisms != nil) {
		return errors.New("follow-up and prevention expected fields require a followup_prompt")
	}
	var prevention []string
	if e.PreventionMechanisms != nil {
		prevention = *e.PreventionMechanisms
	}
	seen := map[string]bool{}
	for _, m := range prevention {
		if !mechanisms[m] {
			return fmt.Errorf("invalid prevention mechanism %q", m)
		}
		if seen[m] {
			return fmt.Errorf("duplicate prevention mechanism %q", m)
		}
		seen[m] = true
	}
	if e.SteeringReason != "" && e.FollowupLabel != "" && e.FollowupLabel != "steering" {
		return errors.New("steering_reason requires followup_label=steering")
	}
	if e.FollowupLabel != "" && e.FollowupLabel != "steering" && (e.PromptIssue != "" || e.AgentsRule != "" || e.SkillCandidate != "" || e.ValidationType != "" || len(prevention) != 0) {
		return errors.New("steering-only expected fields require followup_label=steering")
	}
	if e.PreventionMechanisms == nil {
		return nil
	}
	for _, conditional := range []struct {
		mechanism string
		present   bool
	}{
		{"task_prompt", e.PromptIssue != ""}, {"agents_md", e.AgentsRule != ""}, {"skill", e.SkillCandidate != ""}, {"validation", e.ValidationType != ""},
	} {
		if conditional.present && !contains(prevention, conditional.mechanism) {
			return fmt.Errorf("%s expected field requires matching prevention mechanism", conditional.mechanism)
		}
	}
	return nil
}

func Read(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
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
			return Fixture{}, errors.New("golden fixture contains trailing JSON")
		}
		return Fixture{}, fmt.Errorf("decode trailing golden fixture data: %w", err)
	}
	return fixture, nil
}

func Write(path string, fixture Fixture) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("golden output path is required")
	}
	if fixture.FormatVersion == "" {
		fixture.FormatVersion = FormatVersion
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("encode golden fixture: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".golden-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
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
	head := limit / 2
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
		return analyze.SemanticAnalysis{}, Metrics{}, err
	}
	if workers < 1 || workers > 32 {
		return analyze.SemanticAnalysis{}, Metrics{}, fmt.Errorf("invalid evaluation concurrency %d", workers)
	}
	config := analyze.DefaultSemanticConfig()
	config.Runner = runner
	config.Cache = nil
	config.Workers = workers
	config.Progress = progress
	analysisResult, err := analyze.NewSemanticEngine(config).Analyze(Inputs(fixture))
	if err != nil {
		return analyze.SemanticAnalysis{}, Metrics{}, err
	}
	metrics, err := EvaluateMetrics(fixture, analysisResult.Results)
	if err != nil {
		return analyze.SemanticAnalysis{}, Metrics{}, err
	}
	return analysisResult, metrics, nil
}

func EvaluateMetrics(f Fixture, results []analyze.SemanticResult) (Metrics, error) {
	if len(f.Cases) != len(results) {
		return Metrics{}, fmt.Errorf("metrics received %d results for %d cases", len(results), len(f.Cases))
	}
	byID := make(map[string]analyze.SemanticResult, len(results))
	for _, result := range results {
		if _, duplicate := byID[result.TurnID]; duplicate {
			return Metrics{}, fmt.Errorf("metrics received duplicate result id %q", result.TurnID)
		}
		byID[result.TurnID] = result
	}
	for _, item := range f.Cases {
		if _, ok := byID[item.ID]; !ok {
			return Metrics{}, fmt.Errorf("metrics missing result for case %q", item.ID)
		}
	}
	metrics := Metrics{Fields: make(map[string]FieldMetrics)}
	confusionCounts := make(map[string]int)
	for _, c := range f.Cases {
		if c.Expected == nil {
			continue
		}
		p := byID[c.ID]
		e := c.Expected
		fields := []struct{ name, expected, predicted string }{
			{"task_type", e.TaskType, p.TaskType}, {"followup_label", e.FollowupLabel, p.FollowupLabel}, {"steering_reason", e.SteeringReason, p.SteeringReason}, {"prompt_issue", e.PromptIssue, p.PromptIssue}, {"agents_rule", e.AgentsRule, p.AgentsRule}, {"skill_candidate", e.SkillCandidate, p.SkillCandidate}, {"validation_type", e.ValidationType, p.ValidationType},
		}
		for _, field := range fields {
			if field.expected == "" {
				continue
			}
			stat := metrics.Fields[field.name]
			stat.Samples++
			if field.expected == field.predicted {
				stat.Correct++
			} else {
				confusionCounts[field.name+"\x00"+field.expected+"\x00"+field.predicted]++
			}
			metrics.Fields[field.name] = stat
		}
		if e.PreventionMechanisms != nil {
			metrics.Prevention.Samples++
			expected := unique(*e.PreventionMechanisms)
			predicted := unique(p.PreventionMechanisms)
			if equalStrings(expected, predicted) {
				metrics.Prevention.ExactMatches++
			} else {
				confusionCounts["prevention\x00"+strings.Join(expected, ",")+"\x00"+strings.Join(predicted, ",")]++
			}
			for _, value := range predicted {
				if contains(expected, value) {
					metrics.Prevention.Precision++
				}
			}
			for _, value := range expected {
				if contains(predicted, value) {
					metrics.Prevention.Recall++
				}
			}
		}
	}
	for name, stat := range metrics.Fields {
		if stat.Samples > 0 {
			stat.Accuracy = float64(stat.Correct) / float64(stat.Samples)
			metrics.Fields[name] = stat
		}
	}
	if metrics.Prevention.Samples > 0 {
		predictedTotal, expectedTotal := 0, 0
		for _, c := range f.Cases {
			if c.Expected != nil && c.Expected.PreventionMechanisms != nil {
				predictedTotal += len(unique(byID[c.ID].PreventionMechanisms))
				expectedTotal += len(unique(*c.Expected.PreventionMechanisms))
			}
		}
		if predictedTotal > 0 {
			metrics.Prevention.Precision /= float64(predictedTotal)
		}
		if expectedTotal > 0 {
			metrics.Prevention.Recall /= float64(expectedTotal)
		}
		if metrics.Prevention.Precision+metrics.Prevention.Recall > 0 {
			metrics.Prevention.F1 = 2 * metrics.Prevention.Precision * metrics.Prevention.Recall / (metrics.Prevention.Precision + metrics.Prevention.Recall)
		}
	}
	for key, count := range confusionCounts {
		parts := strings.Split(key, "\x00")
		metrics.Confusions = append(metrics.Confusions, Confusion{Field: parts[0], Expected: parts[1], Predicted: parts[2], Count: count})
	}
	sort.Slice(metrics.Confusions, func(i, j int) bool {
		a, b := metrics.Confusions[i], metrics.Confusions[j]
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Expected != b.Expected {
			return a.Expected < b.Expected
		}
		return a.Predicted < b.Predicted
	})
	return metrics, nil
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
