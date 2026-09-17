package main

import (
	"strings"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func TestGoldenRussianOutputTranslatesSemanticEnums(t *testing.T) {
	tr, err := i18n.New("ru")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ field, raw, translated string }{
		{"task_type", "feature", "Новая функциональность"},
		{"followup_label", "steering", "Корректировка"},
		{"steering_reason", "implementation_error", "Ошибка реализации"},
		{"prompt_issue", "missing_context", "Не хватает контекста"},
	} {
		if got := localizedGoldenValue(tr, item.field, item.raw); got != item.translated || got == item.raw {
			t.Fatalf("%s = %q, want %q", item.field, got, item.translated)
		}
	}
	for _, field := range []string{"task_type", "followup_label", "prevention"} {
		if localizedGoldenField(tr, field) == field {
			t.Fatalf("field key leaked: %s", field)
		}
	}
	for _, key := range []string{"golden_exact", "golden_precision", "golden_recall", "golden_expected", "golden_predicted"} {
		value := tr.T(key)
		if value == key || strings.Contains(value, "exact") || strings.Contains(value, "precision") || strings.Contains(value, "recall") || strings.Contains(value, "expected") || strings.Contains(value, "predicted") {
			t.Fatalf("English metric prose leaked: %q", value)
		}
	}
	line := "refactor cohort and architecture cohort"
	line = localizeHistoricalText(tr, line)
	for _, leaked := range []string{"refactor", "architecture", "cohort", "samples", "observed", "reference", "tolerance", "expected", "pp", "missing", "semantic"} {
		if strings.Contains(line, leaked) {
			t.Fatalf("historical output leaked %q: %s", leaked, line)
		}
	}
	if !strings.Contains(line, "Рефакторинг") || !strings.Contains(line, "Архитектура") {
		t.Fatalf("historical cohort enum leaked: %s", line)
	}
}
