package i18n

func (t Translator) FollowupLabel(value string) string {
	if t.Language == Russian {
		if translated, ok := russianFollowupLabels[value]; ok {
			return translated
		}
	}
	if translated, ok := englishFollowupLabels[value]; ok {
		return translated
	}
	return value
}

var englishFollowupLabels = map[string]string{
	"steering": "Steering", "continuation": "Continuation", "user_correction": "User correction", "question": "Question",
}
var russianFollowupLabels = map[string]string{
	"steering": "Корректировка", "continuation": "Продолжение", "user_correction": "Изменение требования", "question": "Вопрос",
}
