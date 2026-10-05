package i18n

func (t Translator) ValidationType(value string) string {
	english, russian, found := validationTypeTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func validationTypeTranslation(key string) (string, string, bool) {
	switch key {
	case "data_validation":
		return "Data validation", "Проверка данных", true
	case "deployment_check":
		return "Deployment check", "Проверка деплоя", true
	case "diff_review":
		return "Diff review", "Проверка итогового diff", true
	case "other":
		return "Other", "Другое", true
	case "output_validation":
		return "Output validation", "Проверка результата", true
	case "runtime_smoke_check":
		return "Runtime smoke check", "Проверка реального запуска", true
	case "static_analysis":
		return "Static analysis", "Статический анализ", true
	case "tests":
		return "Automated tests", "Автоматические тесты", true
	default:
		return "", "", false
	}
}
