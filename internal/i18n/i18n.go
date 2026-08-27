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
		if value, ok := russianRecommendations[key]; ok {
			return value
		}
	}

	if value, ok := english[key]; ok {
		return value
	}
	if value, ok := englishRecommendations[key]; ok {
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
	"quick_title":                "Codex Insights Quick",
	"analyze_title":              "Codex Insights Analyze",
	"period":                     "Period",
	"all_history":                "all history",
	"last_days":                  "last %d days",
	"user_sessions":              "User sessions",
	"tasks":                      "Tasks",
	"status":                     "Status",
	"complete":                   "complete",
	"aborted":                    "aborted",
	"incomplete":                 "incomplete",
	"completed_averages":         "Completed task averages",
	"tokens":                     "Tokens",
	"duration":                   "Duration",
	"seconds_short":              "sec",
	"tool_calls":                 "Tool calls",
	"model_reasoning":            "Model + reasoning effort",
	"auto_excluded_judges":       "Auto-excluded Codex Insights judge sessions",
	"legacy_excluded":            "Legacy-excluded %s sessions",
	"read_errors":                "Read errors",
	"followup_pairs":             "Follow-up pairs",
	"running_steering":           "Running steering analysis...",
	"running_task_types":         "Running task type analysis...",
	"running_reasons":            "Running steering reason analysis...",
	"judge":                      "LLM evaluation",
	"steering_cache_hits":        "Steering cache hits",
	"steering_new":               "Steering newly evaluated",
	"task_type_cache_hits":       "Task type cache hits",
	"task_type_new":              "Task types newly evaluated",
	"reason_cache_hits":          "Reason cache hits",
	"reason_new":                 "Reasons newly evaluated",
	"behavior":                   "Behavior",
	"analyzed":                   "Analyzed",
	"steering":                   "Steering",
	"continuation":               "Continuation",
	"questions":                  "Questions",
	"user_correction":            "User correction",
	"steering_reasons":           "Steering reasons",
	"task_types":                 "Task types",
	"steering_by_task_type":      "Steering by task type",
	"followups":                  "follow-ups",
	"nothing_to_analyze":         "Nothing to analyze.",
	"effectiveness":              "Model and reasoning effectiveness",
	"effectiveness_insufficient": "Insufficient cohort evidence for a comparison (minimum 10 samples per cohort).",
	"effectiveness_caveat":       "These are associations, not causal effects; harder tasks may be routed to higher reasoning levels.",
	"subagent_effectiveness":     "Subagent effectiveness",
	"subagent_selection_bias":    "Selection bias warning: difficult tasks are more likely to use subagents, so this comparison does not establish that subagents help or hurt.",
	"with_subagents":             "With subagents",
	"without_subagents":          "Without subagents",
	"samples":                    "samples",
	"steering_rate":              "steering",
	"average_tokens":             "avg tokens",
	"average_seconds":            "avg seconds",
	"average_tool_calls":         "avg tool calls",
	"insights_summary":           "Summary",
	"insights_strengths":         "Strengths",
	"insights_weaknesses":        "Weaknesses",
	"insights_high":              "High-priority recommendations",
	"insights_medium":            "Medium-priority recommendations",
	"insights_low":               "Low-priority recommendations",
	"insights_limited":           "Evidence is limited; no reliable cohort comparison is available yet.",
}

var russian = map[string]string{
	"quick_title":                "Codex Insights: быстрый отчёт",
	"analyze_title":              "Codex Insights: глубокий анализ",
	"period":                     "Период",
	"all_history":                "вся история",
	"last_days":                  "последние %d дней",
	"user_sessions":              "Пользовательские сессии",
	"tasks":                      "Задачи",
	"status":                     "Статус",
	"complete":                   "завершено",
	"aborted":                    "прервано",
	"incomplete":                 "не завершено",
	"completed_averages":         "Средние значения завершённых задач",
	"tokens":                     "Токены",
	"duration":                   "Длительность",
	"seconds_short":              "с",
	"tool_calls":                 "Вызовы инструментов",
	"model_reasoning":            "Модель + глубина рассуждений",
	"auto_excluded_judges":       "Автоматически исключено сессий LLM-оценщика Codex Insights",
	"legacy_excluded":            "Исключено старых сессий %s",
	"read_errors":                "Ошибки чтения",
	"followup_pairs":             "Последующие реплики",
	"running_steering":           "Анализируем корректировки Codex...",
	"running_task_types":         "Классифицируем типы задач...",
	"running_reasons":            "Анализируем причины корректировок...",
	"judge":                      "LLM-оценка",
	"steering_cache_hits":        "Корректировок из кеша",
	"steering_new":               "Новых оценок корректировок",
	"task_type_cache_hits":       "Типов задач из кеша",
	"task_type_new":              "Новых типов задач",
	"reason_cache_hits":          "Причин из кеша",
	"reason_new":                 "Новых причин",
	"behavior":                   "Взаимодействие с Codex",
	"analyzed":                   "Проанализировано",
	"steering":                   "Корректировки Codex",
	"continuation":               "Продолжение работы",
	"questions":                  "Вопросы и уточнения",
	"user_correction":            "Изменение требований пользователем",
	"steering_reasons":           "Причины корректировок",
	"task_types":                 "Типы задач",
	"steering_by_task_type":      "Корректировки по типам задач",
	"followups":                  "последующих реплик",
	"nothing_to_analyze":         "Нет данных для анализа.",
	"effectiveness":              "Эффективность модели и глубины рассуждений",
	"effectiveness_insufficient": "Недостаточно данных для сравнения когорт (минимум 10 наблюдений в каждой).",
	"effectiveness_caveat":       "Это взаимосвязь, а не причинный эффект: более сложные задачи могут направляться на более глубокие уровни рассуждений.",
	"subagent_effectiveness":     "Эффективность субагентов",
	"subagent_selection_bias":    "Предупреждение о смещении выборки: сложные задачи чаще используют субагентов, поэтому сравнение не доказывает, что субагенты помогают или вредят.",
	"with_subagents":             "С субагентами",
	"without_subagents":          "Без субагентов",
	"samples":                    "наблюдений",
	"steering_rate":              "корректировки",
	"average_tokens":             "средние токены",
	"average_seconds":            "средние секунды",
	"average_tool_calls":         "средние вызовы инструментов",
	"insights_summary":           "Итог",
	"insights_strengths":         "Сильные стороны",
	"insights_weaknesses":        "Слабые стороны",
	"insights_high":              "Рекомендации высокого приоритета",
	"insights_medium":            "Рекомендации среднего приоритета",
	"insights_low":               "Рекомендации низкого приоритета",
	"insights_limited":           "Данных мало: надёжное сравнение когорт пока невозможно.",
}

var englishTaskTypes = map[string]string{
	"unknown":       "Unknown task type",
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
	"unknown":       "Неизвестный тип задачи",
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

var englishRecommendations = map[string]string{
	"running_prevention":          "Running prevention analysis...",
	"running_validation":          "Running validation gap analysis...",
	"running_prompt_quality":      "Analyzing prompt quality...",
	"running_agents_rules":        "Analyzing AGENTS.md recommendations...",
	"running_skill_candidates":    "Finding reusable Skill candidates...",
	"prevention_cache_hits":       "Prevention cache hits",
	"prevention_new":              "Prevention newly evaluated",
	"validation_cache_hits":       "Validation cache hits",
	"validation_new":              "Validation newly evaluated",
	"prompt_quality_cache_hits":   "Prompt quality cache hits",
	"prompt_quality_new":          "Prompt quality newly evaluated",
	"agents_rules_cache_hits":     "AGENTS.md cache hits",
	"agents_rules_new":            "AGENTS.md newly evaluated",
	"skill_candidates_cache_hits": "Skill candidate cache hits",
	"skill_candidates_new":        "Skill candidates newly evaluated",
	"prompt_quality":              "Prompt quality improvements",
	"agents_recommendations":      "Concrete AGENTS.md recommendations",
	"skill_candidates":            "Reusable Skill candidates",
	"supporting_cases":            "supporting cases",
	"usefulness":                  "Usefulness",
}

var russianRecommendations = map[string]string{
	"running_prevention":          "Анализируем способы снижения корректировок...",
	"running_validation":          "Анализируем недостающие проверки...",
	"running_prompt_quality":      "Анализируем качество постановок...",
	"running_agents_rules":        "Формируем рекомендации для AGENTS.md...",
	"running_skill_candidates":    "Ищем переиспользуемые Skill...",
	"prevention_cache_hits":       "Способов предотвращения из кеша",
	"prevention_new":              "Новых оценок предотвращения",
	"validation_cache_hits":       "Проверок из кеша",
	"validation_new":              "Новых оценок проверок",
	"prompt_quality_cache_hits":   "Постановок из кеша",
	"prompt_quality_new":          "Новых оценок постановок",
	"agents_rules_cache_hits":     "Правил AGENTS.md из кеша",
	"agents_rules_new":            "Новых оценок правил AGENTS.md",
	"skill_candidates_cache_hits": "Кандидатов Skill из кеша",
	"skill_candidates_new":        "Новых оценок кандидатов Skill",
	"prompt_quality":              "Как улучшить постановку задачи",
	"agents_recommendations":      "Конкретные рекомендации для AGENTS.md",
	"skill_candidates":            "Переиспользуемые кандидаты Skill",
	"supporting_cases":            "подтверждающих случаев",
	"usefulness":                  "Польза",
}
