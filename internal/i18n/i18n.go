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
	"quick_title":                       "Codex Insights Quick",
	"analyze_title":                     "Codex Insights Analyze",
	"period":                            "Period",
	"all_history":                       "all history",
	"last_days":                         "last %d days",
	"user_sessions":                     "User sessions",
	"tasks":                             "Tasks",
	"status":                            "Status",
	"complete":                          "complete",
	"aborted":                           "aborted",
	"incomplete":                        "incomplete",
	"completed_averages":                "Completed task averages",
	"tokens":                            "Tokens",
	"duration":                          "Duration",
	"seconds_short":                     "sec",
	"tool_calls":                        "Tool calls",
	"model_reasoning":                   "Model + reasoning effort",
	"auto_excluded_judges":              "Auto-excluded Codex Insights judge sessions",
	"legacy_excluded":                   "Legacy-excluded %s sessions",
	"read_errors":                       "Read errors",
	"followup_pairs":                    "Follow-up pairs",
	"running_steering":                  "Running steering analysis...",
	"running_task_types":                "Running task type analysis...",
	"running_reasons":                   "Running steering reason analysis...",
	"judge":                             "LLM evaluation",
	"steering_cache_hits":               "Steering cache hits",
	"steering_new":                      "Steering newly evaluated",
	"task_type_cache_hits":              "Task type cache hits",
	"task_type_new":                     "Task types newly evaluated",
	"reason_cache_hits":                 "Reason cache hits",
	"reason_new":                        "Reasons newly evaluated",
	"behavior":                          "Behavior",
	"analyzed":                          "Analyzed",
	"steering":                          "Steering",
	"continuation":                      "Continuation",
	"questions":                         "Questions",
	"user_correction":                   "User correction",
	"steering_reasons":                  "Steering reasons",
	"task_types":                        "Task types",
	"steering_by_task_type":             "Steering by task type",
	"followups":                         "follow-ups",
	"nothing_to_analyze":                "Nothing to analyze.",
	"semantic_privacy":                  "Selected task prompts, final answers, and follow-ups will be sent to the configured Judge; the persistent semantic cache stores labels, not raw conversations.",
	"semantic_progress":                 "Semantic analysis",
	"semantic_workers":                  "workers",
	"semantic_cache_hits_short":         "cache hits",
	"semantic_elapsed":                  "elapsed",
	"semantic_cache_hits":               "Semantic cache hits",
	"semantic_new":                      "Semantic records newly evaluated",
	"semantic_methodology":              "Methodology",
	"semantic_model":                    "Model / effort",
	"semantic_prompt_version":           "Prompt version",
	"semantic_schema_version":           "Schema version",
	"timing_discovery":                  "session discovery",
	"timing_parsing":                    "local parsing",
	"timing_cache_lookup":               "cache lookup",
	"timing_judge":                      "semantic Judge",
	"timing_conversion":                 "result conversion",
	"timing_aggregation":                "aggregation",
	"timing_report":                     "final report",
	"effectiveness":                     "Model and reasoning effectiveness",
	"effectiveness_insufficient":        "Insufficient cohort evidence for a comparison (minimum 10 samples per cohort).",
	"effectiveness_caveat":              "These are associations, not causal effects; harder tasks may be routed to higher reasoning levels.",
	"subagent_effectiveness":            "Subagent effectiveness",
	"subagent_selection_bias":           "Selection bias warning: difficult tasks are more likely to use subagents, so this comparison does not establish that subagents help or hurt.",
	"with_subagents":                    "With subagents",
	"without_subagents":                 "Without subagents",
	"samples":                           "samples",
	"steering_rate":                     "steering",
	"average_tokens":                    "avg tokens",
	"average_seconds":                   "avg seconds",
	"average_tool_calls":                "avg tool calls",
	"insights_summary":                  "Summary",
	"insights_strengths":                "Strengths",
	"insights_weaknesses":               "Weaknesses",
	"insights_high":                     "High-priority recommendations",
	"insights_medium":                   "Medium-priority recommendations",
	"insights_low":                      "Low-priority recommendations",
	"insights_limited":                  "Evidence is limited; no reliable cohort comparison is available yet.",
	"golden_export_title":               "Golden fixture candidate exported",
	"golden_candidate_cases":            "Candidate cases",
	"golden_output":                     "Output",
	"golden_local_only":                 "Only minimized, automatically redacted local data was written; no Judge was called. Review the candidate locally because redaction is not a guarantee of secrecy. Human approval is required before evaluation.",
	"golden_evaluate_title":             "Golden fixture evaluation",
	"golden_field_task_type":            "task type",
	"golden_field_followup":             "follow-up label",
	"golden_field_steering_reason":      "steering reason",
	"golden_field_prompt_issue":         "prompt issue",
	"golden_field_agents_rule":          "AGENTS.md rule",
	"golden_field_skill_candidate":      "Skill candidate",
	"golden_field_validation_type":      "validation type",
	"golden_field_prevention":           "prevention mechanisms",
	"golden_samples":                    "samples",
	"golden_exact":                      "exact matches",
	"golden_precision":                  "precision",
	"golden_recall":                     "recall",
	"golden_f1":                         "F1",
	"golden_expected":                   "expected",
	"golden_predicted":                  "predicted",
	"golden_methodology_warning":        "Warning: fixture methodology differs from current (%s / %s / %s vs %s / %s / %s); evaluation uses the current methodology.",
	"historical_guard":                  "Historical aggregate regression guard",
	"historical_pass":                   "PASS",
	"historical_warning":                "WARNING (semantic drift)",
	"historical_fail":                   "FAIL",
	"historical_deterministic_mismatch": "deterministic mismatch",
	"historical_semantic_warning":       "semantic tolerance warning",
}

var russian = map[string]string{
	"quick_title":                       "Codex Insights: быстрый отчёт",
	"analyze_title":                     "Codex Insights: глубокий анализ",
	"period":                            "Период",
	"all_history":                       "вся история",
	"last_days":                         "последние %d дней",
	"user_sessions":                     "Пользовательские сессии",
	"tasks":                             "Задачи",
	"status":                            "Статус",
	"complete":                          "завершено",
	"aborted":                           "прервано",
	"incomplete":                        "не завершено",
	"completed_averages":                "Средние значения завершённых задач",
	"tokens":                            "Токены",
	"duration":                          "Длительность",
	"seconds_short":                     "с",
	"tool_calls":                        "Вызовы инструментов",
	"model_reasoning":                   "Модель + глубина рассуждений",
	"auto_excluded_judges":              "Автоматически исключено сессий LLM-оценщика Codex Insights",
	"legacy_excluded":                   "Исключено старых сессий %s",
	"read_errors":                       "Ошибки чтения",
	"followup_pairs":                    "Последующие реплики",
	"running_steering":                  "Анализируем корректировки Codex...",
	"running_task_types":                "Классифицируем типы задач...",
	"running_reasons":                   "Анализируем причины корректировок...",
	"judge":                             "LLM-оценка",
	"steering_cache_hits":               "Корректировок из кеша",
	"steering_new":                      "Новых оценок корректировок",
	"task_type_cache_hits":              "Типов задач из кеша",
	"task_type_new":                     "Новых типов задач",
	"reason_cache_hits":                 "Причин из кеша",
	"reason_new":                        "Новых причин",
	"behavior":                          "Взаимодействие с Codex",
	"analyzed":                          "Проанализировано",
	"steering":                          "Корректировки Codex",
	"continuation":                      "Продолжение работы",
	"questions":                         "Вопросы и уточнения",
	"user_correction":                   "Изменение требований пользователем",
	"steering_reasons":                  "Причины корректировок",
	"task_types":                        "Типы задач",
	"steering_by_task_type":             "Корректировки по типам задач",
	"followups":                         "последующих реплик",
	"nothing_to_analyze":                "Нет данных для анализа.",
	"semantic_privacy":                  "Выбранные постановки задач, финальные ответы и последующие реплики будут отправлены настроенному оценщику; постоянный семантический кеш хранит метки, а не исходные диалоги.",
	"semantic_progress":                 "Семантический анализ",
	"semantic_workers":                  "воркеры",
	"semantic_cache_hits_short":         "из кеша",
	"semantic_elapsed":                  "время",
	"semantic_cache_hits":               "Семантических результатов из кеша",
	"semantic_new":                      "Новых семантических оценок",
	"semantic_methodology":              "Методология",
	"semantic_model":                    "Модель / глубина",
	"semantic_prompt_version":           "Версия промпта",
	"semantic_schema_version":           "Версия схемы",
	"timing_discovery":                  "поиск сессий",
	"timing_parsing":                    "локальный разбор",
	"timing_cache_lookup":               "поиск в кеше",
	"timing_judge":                      "семантический оценщик",
	"timing_conversion":                 "преобразование результатов",
	"timing_aggregation":                "агрегация",
	"timing_report":                     "итоговый отчёт",
	"effectiveness":                     "Эффективность модели и глубины рассуждений",
	"effectiveness_insufficient":        "Недостаточно данных для сравнения когорт (минимум 10 наблюдений в каждой).",
	"effectiveness_caveat":              "Это взаимосвязь, а не причинный эффект: более сложные задачи могут направляться на более глубокие уровни рассуждений.",
	"subagent_effectiveness":            "Эффективность субагентов",
	"subagent_selection_bias":           "Предупреждение о смещении выборки: сложные задачи чаще используют субагентов, поэтому сравнение не доказывает, что субагенты помогают или вредят.",
	"with_subagents":                    "С субагентами",
	"without_subagents":                 "Без субагентов",
	"samples":                           "наблюдений",
	"steering_rate":                     "корректировки",
	"average_tokens":                    "средние токены",
	"average_seconds":                   "средние секунды",
	"average_tool_calls":                "средние вызовы инструментов",
	"insights_summary":                  "Итог",
	"insights_strengths":                "Сильные стороны",
	"insights_weaknesses":               "Слабые стороны",
	"insights_high":                     "Рекомендации высокого приоритета",
	"insights_medium":                   "Рекомендации среднего приоритета",
	"insights_low":                      "Рекомендации низкого приоритета",
	"insights_limited":                  "Данных мало: надёжное сравнение когорт пока невозможно.",
	"golden_export_title":               "Кандидат golden-фикстуры экспортирован",
	"golden_candidate_cases":            "Кандидатов",
	"golden_output":                     "Файл",
	"golden_local_only":                 "Записаны только минимизированные локальные данные с автоматической очисткой; оценщик не вызывался. Проверьте кандидат локально: очистка не гарантирует удаления всех секретов. Перед оценкой требуется одобрение человека.",
	"golden_evaluate_title":             "Оценка golden-фикстуры",
	"golden_field_task_type":            "тип задачи",
	"golden_field_followup":             "метка продолжения",
	"golden_field_steering_reason":      "причина корректировки",
	"golden_field_prompt_issue":         "проблема постановки",
	"golden_field_agents_rule":          "правило AGENTS.md",
	"golden_field_skill_candidate":      "кандидат Skill",
	"golden_field_validation_type":      "тип проверки",
	"golden_field_prevention":           "механизмы предотвращения",
	"golden_samples":                    "наблюдений",
	"golden_exact":                      "точных совпадений",
	"golden_precision":                  "точность",
	"golden_recall":                     "полнота",
	"golden_f1":                         "F1",
	"golden_expected":                   "ожидалось",
	"golden_predicted":                  "получено",
	"golden_methodology_warning":        "Внимание: методология фикстуры отличается от текущей (%s / %s / %s вместо %s / %s / %s); используется текущая методология.",
	"historical_guard":                  "Проверка исторического агрегированного эталона",
	"historical_pass":                   "ПРОЙДЕНО",
	"historical_warning":                "ПРЕДУПРЕЖДЕНИЕ (семантический дрейф)",
	"historical_fail":                   "ОШИБКА",
	"historical_deterministic_mismatch": "расхождение детерминированных данных",
	"historical_semantic_warning":       "предупреждение о допуске семантики",
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
