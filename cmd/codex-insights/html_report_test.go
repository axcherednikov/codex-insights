package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func TestRenderHTMLReportProducesStandaloneFriendlyEnglishReport(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	input := HTMLReportInput{
		Translator:  tr,
		WindowLabel: "last 30 days",
		Counts:      HTMLReportCounts{UserSessions: 4, Tasks: 12, FollowupPairs: 9},
		Effectiveness: analyze.EffectivenessAnalysis{
			Overall:       analyze.EffectivenessStats{Samples: 12, Steering: 3, AverageTokens: 1200, AverageSeconds: 18, AverageToolCalls: 4},
			TaskTypeStats: []analyze.TaskTypeEffectiveness{{TaskType: "feature", Stats: analyze.EffectivenessStats{Samples: 12, Steering: 3}}},
		},
		Labels: HTMLReportLabels{
			Steering:   []HTMLLabelCount{{Label: "steering", Count: 3}, {Label: "continuation", Count: 9}},
			Reasons:    []HTMLLabelCount{{Label: "insufficient_validation", Count: 3}},
			Validation: []HTMLLabelCount{{Label: "tests", Count: 3}},
		},
	}

	html, err := RenderHTMLReport(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<!doctype html>", "<html", "<meta name=\"viewport\"", "Overview", "Strengths", "Growth areas", "Recommendations", "Supporting metrics", "Methodology", "privacy",
		"3", "Synthetic example", "Before", "After", "What was observed", "Why it matters", "Practical checklist",
		"12", "Automated tests",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML report missing %q", want)
		}
	}
	if strings.Contains(html, "turn-1") || strings.Contains(html, "PreviousTurnID") {
		t.Error("HTML report rendered an internal turn identifier")
	}
}

func TestRenderHTMLReportLocalizesFriendlyContentAndSyntheticExamples(t *testing.T) {
	tr, err := i18n.New("ru")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator:  tr,
		WindowLabel: "последние 30 дней",
		Counts:      HTMLReportCounts{Tasks: 12, FollowupPairs: 1},
		Effectiveness: analyze.EffectivenessAnalysis{
			Overall: analyze.EffectivenessStats{Samples: 12, Steering: 1},
		},
		Labels: HTMLReportLabels{
			Steering: []HTMLLabelCount{{Label: "steering", Count: 1}},
			Reasons:  []HTMLLabelCount{{Label: "implementation_error", Count: 1}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Обзор", "Сильные стороны", "Зоны роста", "Рекомендации", "Методология и приватность", "Синтетический пример", "До", "После", "Практический чек-лист"} {
		if !strings.Contains(html, want) {
			t.Errorf("Russian HTML report missing %q", want)
		}
	}
}

func TestRenderHTMLReportInsufficientDataIsHonest(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{Translator: tr})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Not enough evidence", "No reliable conclusions", "Collect more completed tasks"} {
		if !strings.Contains(html, want) {
			t.Errorf("insufficient-data report missing %q", want)
		}
	}
}

func TestRenderHTMLReportEscapesAggregateLabels(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator:    tr,
		Effectiveness: analyze.EffectivenessAnalysis{Overall: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize}},
		Labels:        HTMLReportLabels{Reasons: []HTMLLabelCount{{Label: "<script>alert('x')</script>", Count: 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>alert") || !strings.Contains(html, "&lt;script&gt;") {
		t.Error("aggregate label was not safely escaped")
	}
}

func TestHTMLReportInputHasNoRawConversationFields(t *testing.T) {
	typ := reflect.TypeOf(HTMLReportInput{})
	allowed := map[string]bool{"translator": true, "windowlabel": true, "windowstart": true, "windowend": true, "counts": true, "semantic": true, "effectiveness": true, "labels": true}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if !allowed[name] {
			t.Fatalf("HTMLReportInput has unexpected field %q", typ.Field(i).Name)
		}
		assertNoRawConversationFields(t, typ.Field(i).Type, map[reflect.Type]bool{})
	}
	// This compile-time construction intentionally uses only aggregate fields;
	// the input type has no prompt, answer, interaction, turn, or follow-up text.
	_ = HTMLReportInput{Counts: HTMLReportCounts{Tasks: 1}, Labels: HTMLReportLabels{Steering: []HTMLLabelCount{{Label: "continuation", Count: 1}}}}
}

func assertNoRawConversationFields(t *testing.T, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.PkgPath() == "time" || typ.PkgPath() == "github.com/axcherednikov/codex-insights/internal/i18n" || seen[typ] || typ.Kind() != reflect.Struct {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.ToLower(field.Name)
		if name == "prompt" || name == "answer" || name == "session" || name == "interaction" || name == "followup" || name == "conversation" || name == "turnid" || strings.Contains(name, "previousturn") {
			t.Fatalf("HTMLReportInput reaches raw conversation field %q in %s", field.Name, typ)
		}
		assertNoRawConversationFields(t, field.Type, seen)
	}
}

func TestRenderHTMLReportIsDeterministic(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	input := HTMLReportInput{
		Translator:    tr,
		Counts:        HTMLReportCounts{Tasks: 2},
		Effectiveness: analyze.EffectivenessAnalysis{Overall: analyze.EffectivenessStats{Samples: 2}},
		Labels:        HTMLReportLabels{Steering: []HTMLLabelCount{{Label: "steering", Count: 1}}, Reasons: []HTMLLabelCount{{Label: "implementation_error", Count: 1}}},
	}
	first, err := RenderHTMLReport(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderHTMLReport(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("same aggregate input produced different HTML output")
	}
}

func TestRenderHTMLReportBelowEvidenceThresholdDoesNotConclude(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator:    tr,
		Effectiveness: analyze.EffectivenessAnalysis{Overall: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize - 1, Steering: 1}},
		Labels:        HTMLReportLabels{Steering: []HTMLLabelCount{{Label: "steering", Count: 1}}, Reasons: []HTMLLabelCount{{Label: "implementation_error", Count: 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Not enough evidence") || strings.Contains(html, "Address the most common correction cause") {
		t.Error("below-threshold evidence produced a confident conclusion")
	}
}

func TestRenderHTMLReportDoesNotUseOneCohortAsBothStrengthAndWeakness(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator: tr,
		Effectiveness: analyze.EffectivenessAnalysis{
			Overall:       analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize},
			TaskTypeStats: []analyze.TaskTypeEffectiveness{{TaskType: "feature", Stats: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize, Steering: 2}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "lowest observed steering rate") || strings.Contains(html, "highest observed steering rate") {
		t.Error("one cohort was presented as a reliable strength or weakness")
	}
}

func TestRenderHTMLReportDoesNotClaimTiedCohortsDiffer(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator: tr,
		Effectiveness: analyze.EffectivenessAnalysis{
			Overall: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize * 2},
			TaskTypeStats: []analyze.TaskTypeEffectiveness{
				{TaskType: "feature", Stats: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize, Steering: 2}},
				{TaskType: "bugfix", Stats: analyze.EffectivenessStats{Samples: analyze.MinimumCohortSize, Steering: 2}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "lowest observed steering rate") || strings.Contains(html, "highest observed steering rate") {
		t.Error("tied cohorts were presented as a reliable strength or weakness")
	}
}

func TestRenderHTMLReportLocalizedEmptyRecommendations(t *testing.T) {
	for _, language := range []string{"en", "ru"} {
		tr, err := i18n.New(language)
		if err != nil {
			t.Fatal(err)
		}
		html, err := RenderHTMLReport(HTMLReportInput{Translator: tr})
		if err != nil {
			t.Fatal(err)
		}
		if language == "en" && !strings.Contains(html, "No actionable recommendation is supported") {
			t.Error("English empty recommendation state missing")
		}
		if language == "ru" && !strings.Contains(html, "Нет рекомендаций") {
			t.Error("Russian empty recommendation state missing")
		}
	}
}

func TestRenderHTMLReportSteeringRateUsesAllBehaviorLabelsWhenEffectivenessIsAbsent(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLReport(HTMLReportInput{
		Translator: tr,
		Labels: HTMLReportLabels{Steering: []HTMLLabelCount{
			{Label: "steering", Count: 2},
			{Label: "continuation", Count: 8},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "20.0%") {
		t.Errorf("steering metric should use all behavior labels as denominator; HTML=%q", html)
	}
}
