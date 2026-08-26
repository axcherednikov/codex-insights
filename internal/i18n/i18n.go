package i18n

import (
	"fmt"
	"os"
	"strings"
)

type Language string

const (
	English Language = "en"
	Russian Language = "ru"
	Auto    Language = "auto"
)

type Translator struct {
	Language Language
}

func New(value string) (Translator, error) {
	language := Language(strings.ToLower(value))

	switch language {
	case "", Auto:
		language = detectLanguage()
	case English, Russian:
	default:
		return Translator{}, fmt.Errorf(
			"unsupported language %q; supported: auto, en, ru",
			value,
		)
	}

	return Translator{Language: language}, nil
}

func detectLanguage() Language {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.ToLower(os.Getenv(key))

		if strings.HasPrefix(value, "ru") {
			return Russian
		}

		if strings.HasPrefix(value, "en") {
			return English
		}
	}

	return English
}

func (t Translator) T(key string) string {
	if t.Language == Russian {
		if value, ok := russian[key]; ok {
			return value
		}
	}

	if value, ok := english[key]; ok {
		return value
	}

	return key
}

func (t Translator) TaskType(value string) string {
	if t.Language == Russian {
		if translated, ok := russianTaskTypes[value]; ok {
			return translated
		}
	}

	if translated, ok := englishTaskTypes[value]; ok {
		return translated
	}

	return value
}

func (t Translator) SteeringReason(value string) string {
	if t.Language == Russian {
		if translated, ok := russianSteeringReasons[value]; ok {
			return translated
		}
	}

	if translated, ok := englishSteeringReasons[value]; ok {
		return translated
	}

	return value
}

var english = map[string]string{
	"quick_title":           "Codex Insights Quick",
	"analyze_title":         "Codex Insights Analyze",
	"period":                "Period",
	"all_history":           "all history",
	"last_days":             "last %d days",
	"user_sessions":         "User sessions",
	"tasks":                 "Tasks",
	"status":                "Status",
	"complete":              "complete",
	"aborted":               "aborted",
	"incomplete":            "incomplete",
	"completed_averages":    "Completed task averages",
	"tokens":                "Tokens",
	"duration":              "Duration",
	"seconds_short":         "sec",
	"tool_calls":            "Tool calls",
	"model_reasoning":       "Model + reasoning effort",
	"auto_excluded_judges":  "Auto-excluded Codex Insights judge sessions",
	"legacy_excluded":       "Legacy-excluded %s sessions",
	"read_errors":           "Read errors",
	"followup_pairs":        "Follow-up pairs",
	"running_steering":      "Running steering analysis...",
	"running_task_types":    "Running task type analysis...",
	"running_reasons":       "Running steering reason analysis...",
	"judge":                 "LLM evaluation",
	"steering_cache_hits":   "Steering cache hits",
	"steering_new":          "Steering newly evaluated",
	"task_type_cache_hits":  "Task type cache hits",
	"task_type_new":         "Task types newly evaluated",
	"reason_cache_hits":     "Reason cache hits",
	"reason_new":            "Reasons newly evaluated",
	"behavior":              "Behavior",
	"analyzed":              "Analyzed",
	"steering":              "Steering",
	"continuation":          "Continuation",
	"questions":             "Questions",
	"user_correction":       "User correction",
	"steering_reasons":      "Steering reasons",
	"task_types":            "Task types",
	"steering_by_task_type": "Steering by task type",
	"followups":             "follow-ups",
	"nothing_to_analyze":    "Nothing to analyze.",
}

var russian = map[string]string{
	"quick_title":           "Codex Insights: быстрый отчёт",
	"analyze_title":         "Codex Insights: глубокий анализ",
	"period":                "Период",
	"all_history":           "вся история",
	"last_days":             "последние %d дней",
	"user_sessions":         "Пользовательские сессии",
	"tasks":                 "Задачи",
	"status":                "Статус",
	"complete":              "завершено",
	"aborted":               "прервано",
	"incomplete":            "не завершено",
	"completed_averages":    "Средние значения завершённых задач",
	"tokens":                "Токены",
	"duration":              "Длительность",
	"seconds_short":         "с",
	"tool_calls":            "Вызовы инструментов",
	"model_reasoning":       "Модель + глубина рассуждений",
	"auto_excluded_judges":  "Автоматически исключено сессий LLM-оценщика Codex Insights",
	"legacy_excluded":       "Исключено старых сессий %s",
	"read_errors":           "Ошибки чтения",
	"followup_pairs":        "Последующие реплики",
	"running_steering":      "Анализируем корректировки Codex...",
	"running_task_types":    "Классифицируем типы задач...",
	"running_reasons":       "Анализируем причины корректировок...",
	"judge":                 "LLM-оценка",
	"steering_cache_hits":   "Корректировок из кеша",
	"steering_new":          "Новых оценок корректировок",
	"task_type_cache_hits":  "Типов задач из кеша",
	"task_type_new":         "Новых типов задач",
	"reason_cache_hits":     "Причин из кеша",
	"reason_new":            "Новых причин",
	"behavior":              "Взаимодействие с Codex",
	"analyzed":              "Проанализировано",
	"steering":              "Корректировки Codex",
	"continuation":          "Продолжение работы",
	"questions":             "Вопросы и уточнения",
	"user_correction":       "Изменение требований пользователем",
	"steering_reasons":      "Причины корректировок",
	"task_types":            "Типы задач",
	"steering_by_task_type": "Корректировки по типам задач",
	"followups":             "последующих реплик",
	"nothing_to_analyze":    "Нет данных для анализа.",
}

var englishTaskTypes = map[string]string{
	"bugfix":        "Bug fix",
	"feature":       "Feature",
	"refactor":      "Refactoring",
	"tests":         "Tests",
	"code_review":   "Code review",
	"architecture":  "Architecture",
	"devops":        "DevOps",
	"research":      "Research",
	"documentation": "Documentation",
	"other":         "Other",
}

var russianTaskTypes = map[string]string{
	"bugfix":        "Исправление ошибок",
	"feature":       "Новая функциональность",
	"refactor":      "Рефакторинг",
	"tests":         "Тесты",
	"code_review":   "Ревью кода",
	"architecture":  "Архитектура",
	"devops":        "DevOps",
	"research":      "Исследование",
	"documentation": "Документация",
	"other":         "Другое",
}

var englishSteeringReasons = map[string]string{
	"misunderstood_request":   "Misunderstood request",
	"implementation_error":    "Implementation error",
	"insufficient_validation": "Insufficient validation",
	"architecture_mismatch":   "Architecture mismatch",
	"overengineering":         "Overengineering",
	"wrong_output_format":     "Wrong output format",
	"ignored_constraints":     "Ignored constraints",
	"other":                   "Other",
}

var russianSteeringReasons = map[string]string{
	"misunderstood_request":   "Неверно понята задача",
	"implementation_error":    "Ошибка реализации",
	"insufficient_validation": "Недостаточная проверка результата",
	"architecture_mismatch":   "Несоответствие архитектуре",
	"overengineering":         "Переусложнение решения",
	"wrong_output_format":     "Неверный формат результата",
	"ignored_constraints":     "Проигнорированы ограничения",
	"other":                   "Другое",
}
