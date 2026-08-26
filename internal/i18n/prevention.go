package i18n

func (t Translator) PreventionMechanism(value string) string {
	if t.Language == Russian {
		if translated, ok := russianPreventionMechanisms[value]; ok {
			return translated
		}
	}

	if translated, ok := englishPreventionMechanisms[value]; ok {
		return translated
	}

	return value
}

var englishPreventionMechanisms = map[string]string{
	"validation":  "Validation",
	"agents_md":   "AGENTS.md rules",
	"task_prompt": "Task prompt",
	"skill":       "Skill",
}

var russianPreventionMechanisms = map[string]string{
	"validation":  "Автоматическая проверка",
	"agents_md":   "Правила AGENTS.md",
	"task_prompt": "Постановка задачи",
	"skill":       "Skill",
}
