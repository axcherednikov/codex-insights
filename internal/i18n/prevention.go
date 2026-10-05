package i18n

func (t Translator) PreventionMechanism(value string) string {
	english, russian, found := preventionMechanismTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func preventionMechanismTranslation(key string) (string, string, bool) {
	switch key {
	case "agents_md":
		return "AGENTS.md rules", "Правила AGENTS.md", true
	case "skill":
		return "Skill", "Skill", true
	case "task_prompt":
		return "Task prompt", "Постановка задачи", true
	case "validation":
		return "Validation", "Автоматическая проверка", true
	default:
		return "", "", false
	}
}
