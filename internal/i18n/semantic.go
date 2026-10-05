package i18n

func (t Translator) FollowupLabel(value string) string {
	english, russian, found := followupLabelTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func followupLabelTranslation(key string) (string, string, bool) {
	switch key {
	case "continuation":
		return "Continuation", "Продолжение", true
	case "question":
		return "Question", "Вопрос", true
	case "steering":
		return "Steering", "Корректировка", true
	case "user_correction":
		return "User correction", "Изменение требования", true
	default:
		return "", "", false
	}
}
