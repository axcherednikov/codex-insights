package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/golden"
	"codex-insights/internal/i18n"
)

const maxGoldenCases = 500

func goldenWindow(sessionsPath, beforeRaw string, days int, legacy string) (string, sessionWindow, error) {
	if sessionsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", sessionWindow{}, err
		}
		sessionsPath = filepath.Join(home, ".codex", "sessions")
	}
	before := time.Now().UTC()
	if beforeRaw != "" {
		parsed, err := time.Parse(time.RFC3339, beforeRaw)
		if err != nil {
			return "", sessionWindow{}, fmt.Errorf("invalid --before: %w", err)
		}
		before = parsed
	}
	var since time.Time
	if days > 0 {
		since = before.Add(-time.Duration(days) * 24 * time.Hour)
	}
	return sessionsPath, sessionWindow{Since: since, Before: before, LegacyExcludeOriginator: legacy}, nil
}

func runGoldenExport(args []string) error {
	fs := flag.NewFlagSet("golden export", flag.ContinueOnError)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sessionsPath := fs.String("sessions", filepath.Join(home, ".codex", "sessions"), "path to Codex sessions")
	days := fs.Int("days", 30, "number of days to scan; 0 means all history")
	beforeRaw := fs.String("before", "", "scan state before this RFC3339 timestamp")
	legacy := fs.String("legacy-exclude-originator", "", "exclude historical sessions by originator")
	limit := fs.Int("limit", 50, "maximum candidate cases (1-500)")
	output := fs.String("output", "", "required output fixture path")
	lang := fs.String("lang", "auto", "report language: auto, en, ru")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return fmt.Errorf("--output is required")
	}
	if *limit < 1 || *limit > maxGoldenCases {
		return fmt.Errorf("invalid --limit %d; must be between 1 and %d", *limit, maxGoldenCases)
	}
	resolvedPath, window, err := goldenWindow(*sessionsPath, *beforeRaw, *days, *legacy)
	if err != nil {
		return err
	}
	collected, err := collectSessions(resolvedPath, window)
	if err != nil {
		return err
	}
	inputs := golden.CollectCandidateInputs(collected.Interactions, collected.Followups)
	defaults := analyze.DefaultSemanticConfig()
	fixture := golden.BuildCandidate(inputs, *limit, golden.Methodology{MethodologyVersion: defaults.MethodologyVersion, PromptVersion: defaults.PromptVersion, SchemaVersion: defaults.SchemaVersion})
	if err := golden.Write(*output, fixture); err != nil {
		return fmt.Errorf("write golden fixture: %w", err)
	}
	tr, err := i18n.New(*lang)
	if err != nil {
		return err
	}
	fmt.Println(tr.T("golden_export_title"))
	fmt.Printf("%s: %d\n", tr.T("golden_candidate_cases"), len(fixture.Cases))
	fmt.Printf("%s: %s\n", tr.T("golden_output"), *output)
	fmt.Println(tr.T("golden_local_only"))
	return nil
}

func runGoldenEvaluate(args []string) error {
	fs := flag.NewFlagSet("golden evaluate", flag.ContinueOnError)
	fixturePath := fs.String("fixture", "", "required approved fixture path")
	concurrency := fs.Int("concurrency", analyze.SemanticDefaultWorkers, "number of semantic Judge workers (1-32)")
	lang := fs.String("lang", "auto", "report language: auto, en, ru")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fixturePath == "" && fs.NArg() == 1 {
		*fixturePath = fs.Arg(0)
	}
	if *fixturePath == "" {
		return fmt.Errorf("--fixture is required")
	}
	if err := validateSemanticConcurrency(*concurrency); err != nil {
		return err
	}
	fixture, err := golden.Read(*fixturePath)
	if err != nil {
		return err
	}
	if err := fixture.ValidateForEvaluation(); err != nil {
		return err
	}
	defaults := analyze.DefaultSemanticConfig()
	tr, err := i18n.New(*lang)
	if err != nil {
		return err
	}
	currentMethodology := golden.Methodology{MethodologyVersion: defaults.MethodologyVersion, PromptVersion: defaults.PromptVersion, SchemaVersion: defaults.SchemaVersion}
	if fixture.Methodology != currentMethodology {
		fmt.Printf("%s\n", fmt.Sprintf(tr.T("golden_methodology_warning"), fixture.Methodology.MethodologyVersion, fixture.Methodology.PromptVersion, fixture.Methodology.SchemaVersion, currentMethodology.MethodologyVersion, currentMethodology.PromptVersion, currentMethodology.SchemaVersion))
	}
	renderer := newSemanticProgressRenderer(tr, os.Stdout, stdoutIsTTY())
	_, metrics, err := golden.Evaluate(fixture, defaults.Runner, *concurrency, renderer.Update)
	if err != nil {
		renderer.Finish()
		return err
	}
	renderer.Finish()
	fmt.Println()
	fmt.Println(tr.T("golden_evaluate_title"))
	fields := make([]string, 0, len(metrics.Fields))
	for field := range metrics.Fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		stat := metrics.Fields[field]
		fmt.Printf("  %s: %d/%d (%.1f%%)\n", localizedGoldenField(tr, field), stat.Correct, stat.Samples, stat.Accuracy*100)
	}
	fmt.Printf("  %s: %d/%d %s, %s %.1f%%, %s %.1f%%, %s %.1f%%\n", tr.T("golden_field_prevention"), metrics.Prevention.ExactMatches, metrics.Prevention.Samples, tr.T("golden_exact"), tr.T("golden_precision"), metrics.Prevention.Precision*100, tr.T("golden_recall"), metrics.Prevention.Recall*100, tr.T("golden_f1"), metrics.Prevention.F1*100)
	for _, line := range metrics.Confusions {
		fmt.Printf("  %s: %s %s -> %s %s (%d %s)\n", localizedGoldenField(tr, line.Field), tr.T("golden_expected"), localizedGoldenValue(tr, line.Field, line.Expected), tr.T("golden_predicted"), localizedGoldenValue(tr, line.Field, line.Predicted), line.Count, tr.T("golden_samples"))
	}
	for _, stat := range metrics.Fields {
		if stat.Correct != stat.Samples {
			return fmt.Errorf("golden regression: approved expectations were not met")
		}
	}
	if metrics.Prevention.Samples > 0 && metrics.Prevention.ExactMatches != metrics.Prevention.Samples {
		return fmt.Errorf("golden regression: prevention expectations were not met")
	}
	return nil
}

func localizedGoldenValue(tr i18n.Translator, field, value string) string {
	switch field {
	case "task_type":
		return tr.TaskType(value)
	case "followup_label":
		return tr.FollowupLabel(value)
	case "steering_reason":
		return tr.SteeringReason(value)
	case "prompt_issue":
		return tr.PromptQualityIssue(value)
	case "agents_rule":
		return tr.AgentsRule(value)
	case "skill_candidate":
		return tr.SkillCandidate(value)
	case "validation_type":
		return tr.ValidationType(value)
	case "prevention":
		parts := strings.Split(value, ",")
		for i, part := range parts {
			if part != "" {
				parts[i] = tr.PreventionMechanism(part)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return value
	}
}

func localizedGoldenField(tr i18n.Translator, field string) string {
	keys := map[string]string{"task_type": "golden_field_task_type", "followup_label": "golden_field_followup", "steering_reason": "golden_field_steering_reason", "prompt_issue": "golden_field_prompt_issue", "agents_rule": "golden_field_agents_rule", "skill_candidate": "golden_field_skill_candidate", "validation_type": "golden_field_validation_type", "prevention": "golden_field_prevention"}
	if key, ok := keys[field]; ok {
		return tr.T(key)
	}
	return field
}
