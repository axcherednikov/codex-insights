package i18n

func (t Translator) ValidationType(value string) string {
	if t.Language == Russian {
		if translated, ok := russianValidationTypes[value]; ok {
			return translated
		}
	}

	if translated, ok := englishValidationTypes[value]; ok {
		return translated
	}

	return value
}

var englishValidationTypes = map[string]string{
	"runtime_smoke_check": "Runtime smoke check",
	"deployment_check":    "Deployment check",
	"diff_review":         "Diff review",
	"static_analysis":     "Static analysis",
	"output_validation":   "Output validation",
	"tests":               "Automated tests",
	"data_validation":     "Data validation",
	"other":               "Other",
}

var russianValidationTypes = map[string]string{
	"runtime_smoke_check": "Проверка реального запуска",
	"deployment_check":    "Проверка деплоя",
	"diff_review":         "Проверка итогового diff",
	"static_analysis":     "Статический анализ",
	"output_validation":   "Проверка результата",
	"tests":               "Автоматические тесты",
	"data_validation":     "Проверка данных",
	"other":               "Другое",
}
