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
		return Translator{}, fmt.Errorf("%w %q; supported: auto, en, ru", errUnsupportedLanguage, value)
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
	english, russian, found := baseTranslation(key)
	if !found {
		english, russian, found = recommendationTextTranslation(key)
	}

	return localized(t.Language, english, russian, found, key)
}

func baseTranslation(key string) (string, string, bool) {
	if english, russian, found := lookupBaseLabels(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupReportLabels(key); found {
		return english, russian, true
	}

	return "", "", false
}

func lookupBaseLabels(key string) (string, string, bool) {
	if english, russian, found := lookupSubagentLabels(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupUsagePeriod(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupUsageStatus(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupReportAverages(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupAnalysisLabels(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupAnalysisSections(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSemanticStatus(key); found {
		return english, russian, true
	}

	return "", "", false
}

func lookupSubagentLabels(key string) (string, string, bool) {
	if english, russian, found := lookupSubagentResourceLabels(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSubagentTokenComponents(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSubagentResourceCoverage(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSubagentActivityLabels(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSubagentActivityCoverage(key); found {
		return english, russian, true
	}

	return "", "", false
}

func lookupSubagentResourceLabels(key string) (string, string, bool) {
	switch key {
	case "subagent_resource_unknown":
		return "n/a", "н/д", true
	case "subagent_resource_overflow":
		return "A resource sum exceeds the supported integer range and is unavailable.", "Сумма расхода превышает поддерживаемый диапазон чисел и недоступна.", true
	case "subagent_recorded_tokens":
		return "Tokens (token_usage_record)", "Токены (token_usage_record)", true
	case "subagent_estimated_tokens":
		return "Estimated tokens (token_count)", "Оценка токенов (token_count)", true
	case "subagent_token_subsets_note":
		return "Cached input is included in input; reasoning is included in output. Sources are shown separately and totals cover confirmed data only; n/a means missing or unconfirmed data.", "Кешированные входят во входные; reasoning — в выходные. Источники показаны отдельно, суммы включают только подтверждённые данные; н/д — отсутствующие или неподтверждённые данные.", true
	case "subagent_tokens_by_role":
		return "Tokens by logged role", "Токены по записанным ролям", true
	case "subagent_tokens_by_model":
		return "Tokens by observed model (unknown = attribution unavailable)", "Токены по фактическим моделям (unknown — принадлежность не подтверждена)", true
	case "subagent_working_time":
		return "Recorded working time", "Записанное рабочее время", true
	case "subagent_working_time_note":
		return "Sum of recorded agent turn durations, including parallel work; not elapsed time of the user task.", "Сумма записанных длительностей ходов агентов, включая параллельную работу; не длительность пользовательской задачи.", true
	default:
		return "", "", false
	}
}

func lookupSubagentTokenComponents(key string) (string, string, bool) {
	switch key {
	case "subagent_input_tokens":
		return "input", "входные", true
	case "subagent_cached_tokens":
		return "including cached input", "из них кешированные", true
	case "subagent_output_tokens":
		return "output", "выходные", true
	case "subagent_reasoning_tokens":
		return "including reasoning output", "из них reasoning", true
	default:
		return "", "", false
	}
}

func lookupSubagentResourceCoverage(key string) (string, string, bool) {
	switch key {
	case "subagent_missing_tokens":
		return "Turns without attributable token data", "Ходы без подтверждённых данных о токенах", true
	case "subagent_invalid_tokens":
		return "Unusable or conflicting request records", "Непригодные или противоречивые записи расхода", true
	case "subagent_invalid_counters":
		return "Turns with ambiguous, invalid or reset token_count counters", "Ходы с неоднозначными, некорректными или сброшенными token_count", true
	case "subagent_counter_mismatches":
		return "Turns where sources disagree (token_usage_record preferred)", "Ходы с расхождением источников (приоритет token_usage_record)", true
	case "subagent_unverifiable_counters":
		return "Recorded turns without comparable token_count", "Ходы с записанным расходом без сопоставимого token_count", true
	case "subagent_missing_time":
		return "Turns without recorded duration", "Ходы без записанной длительности", true
	case "subagent_invalid_time":
		return "Turns with invalid or conflicting duration", "Ходы с некорректной или противоречивой длительностью", true
	default:
		return "", "", false
	}
}

func lookupSubagentActivityLabels(key string) (string, string, bool) {
	switch key {
	case "subagents":
		return "Subagents", "Субагенты", true
	case "subagents_created":
		return "Created", "Создано", true
	case "subagents_working_turns":
		return "Working turns", "Рабочие ходы", true
	case "subagents_completed":
		return "Completed", "Завершено", true
	case "subagents_aborted":
		return "Aborted", "Прервано", true
	case "subagents_incomplete":
		return "Incomplete", "Не завершено", true
	case "subagents_nested":
		return "Nested created", "Вложенные созданные", true
	case "subagents_roles":
		return "Roles", "Роли", true
	case "subagents_created_short":
		return "created", "создано", true
	case "subagents_working_short":
		return "working turns", "рабочие ходы", true
	default:
		return "", "", false
	}
}

func lookupSubagentActivityCoverage(key string) (string, string, bool) {
	switch key {
	case "subagents_settings":
		return "Observed model / effort settings", "Наблюдавшиеся модель / глубина", true
	case "subagents_setting_note":
		return "a turn may appear in multiple settings", "один ход может учитываться в нескольких настройках", true
	case "subagents_unattributed":
		return "Forked starts excluded for missing ownership", "Ходы форка исключены: нет данных о владельце", true
	case "subagents_missing_ids":
		return "Files missing child identity", "Файлы без идентификатора дочернего потока", true
	case "subagents_missing_parents":
		return "Agents missing parent", "Агенты без родителя", true
	case "subagents_missing_depth":
		return "Agents missing logged depth", "Агенты без записанной глубины", true
	case "subagents_uncertain":
		return "Conflicting or invalid records", "Противоречивые или некорректные записи", true
	case "subagents_read_errors":
		return "Subagent read errors", "Ошибки чтения субагентов", true
	case "subagents_malformed":
		return "Malformed or invalid records", "Некорректные записи", true
	case "subagents_terminal_mismatches":
		return "Terminal events without matching starts", "Завершающие события без начала хода", true
	default:
		return "", "", false
	}
}

func lookupReportLabels(key string) (string, string, bool) {
	if english, russian, found := lookupSubagentEffectiveness(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupSemanticTiming(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupEffectivenessCohorts(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupEffectivenessInsights(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupGoldenExport(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupGoldenFields(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupGoldenMetrics(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupHistorical(key); found {
		return english, russian, true
	}

	return "", "", false
}

func localized(language Language, english, russian string, found bool, fallback string) string {
	if !found {
		return fallback
	}

	if language == Russian {
		return russian
	}

	return english
}

func (t Translator) TaskType(value string) string {
	english, russian, found := taskTypeTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func taskTypeTranslation(key string) (string, string, bool) {
	switch key {
	case "architecture":
		return "Architecture", "Архитектура", true
	case "bugfix":
		return "Bug fix", "Исправление ошибок", true
	case "code_review":
		return "Code review", "Ревью кода", true
	case "devops":
		return "DevOps", "DevOps", true
	case "documentation":
		return "Documentation", "Документация", true
	case "feature":
		return "Feature", "Новая функциональность", true
	case "other":
		return "Other", "Другое", true
	case "refactor":
		return "Refactoring", "Рефакторинг", true
	case "research":
		return "Research", "Исследование", true
	case "tests":
		return "Tests", "Тесты", true
	case "unknown":
		return "Unknown task type", "Неизвестный тип задачи", true
	default:
		return "", "", false
	}
}

func (t Translator) SteeringReason(value string) string {
	english, russian, found := steeringReasonTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func steeringReasonTranslation(key string) (string, string, bool) {
	switch key {
	case "architecture_mismatch":
		return "Architecture mismatch", "Несоответствие архитектуре", true
	case "ignored_constraints":
		return "Ignored constraints", "Проигнорированы ограничения", true
	case "implementation_error":
		return "Implementation error", "Ошибка реализации", true
	case "insufficient_validation":
		return "Insufficient validation", "Недостаточная проверка результата", true
	case "misunderstood_request":
		return "Misunderstood request", "Неверно понята задача", true
	case "other":
		return "Other", "Другое", true
	case "overengineering":
		return "Overengineering", "Переусложнение решения", true
	case "wrong_output_format":
		return "Wrong output format", "Неверный формат результата", true
	default:
		return "", "", false
	}
}

func lookupUsagePeriod(key string) (string, string, bool) {
	switch key {
	case "quick_title":
		return "Codex Insights Quick", "Codex Insights: быстрый отчёт", true
	case "analyze_title":
		return "Codex Insights Analyze", "Codex Insights: глубокий анализ", true
	case "period":
		return "Period", "Период", true
	case "all_history":
		return "all history", "вся история", true
	case "last_days":
		return "last %d days", "последние %d дней", true
	default:
		return "", "", false
	}
}

func lookupUsageStatus(key string) (string, string, bool) {
	switch key {
	case "user_sessions":
		return "User sessions", "Пользовательские сессии", true
	case "tasks":
		return "Tasks", "Задачи", true
	case "status":
		return "Status", "Статус", true
	case "complete":
		return "complete", "завершено", true
	case "aborted":
		return "aborted", "прервано", true
	case "incomplete":
		return "incomplete", "не завершено", true
	case "codex_cli_missing":
		return "Codex CLI was not found in PATH. Install it from https://developers.openai.com/codex/cli, then run \"codex\" once to sign in.", "Codex CLI не найден в PATH. Установите его по инструкции https://developers.openai.com/codex/cli, затем один раз запустите \"codex\" и войдите в аккаунт.", true
	default:
		return "", "", false
	}
}

func lookupReportAverages(key string) (string, string, bool) {
	switch key {
	case "completed_averages":
		return "Completed task averages", "Средние значения завершённых задач", true
	case "tokens":
		return "Tokens", "Токены", true
	case "duration":
		return "Duration", "Длительность", true
	case "seconds_short":
		return "sec", "с", true
	case "tool_calls":
		return "Tool calls", "Вызовы инструментов", true
	case "model_reasoning":
		return "Model + reasoning effort", "Модель + глубина рассуждений", true
	case "auto_excluded_judges":
		return "Auto-excluded Codex Insights judge sessions", "Автоматически исключено сессий LLM-оценщика Codex Insights", true
	case "legacy_excluded":
		return "Legacy-excluded %s sessions", "Исключено старых сессий %s", true
	case "read_errors":
		return "Read errors", "Ошибки чтения", true
	case "followup_pairs":
		return "Follow-up pairs", "Последующие реплики", true
	default:
		return "", "", false
	}
}

func lookupAnalysisLabels(key string) (string, string, bool) {
	switch key {
	case "running_steering":
		return "Running steering analysis...", "Анализируем корректировки Codex...", true
	case "running_task_types":
		return "Running task type analysis...", "Классифицируем типы задач...", true
	case "running_reasons":
		return "Running steering reason analysis...", "Анализируем причины корректировок...", true
	case "judge":
		return "LLM evaluation", "LLM-оценка", true
	case "behavior":
		return "Behavior", "Взаимодействие с Codex", true
	case "analyzed":
		return "Analyzed", "Проанализировано", true
	case "steering":
		return "Steering", "Корректировки Codex", true
	case "continuation":
		return "Continuation", "Продолжение работы", true
	case "questions":
		return "Questions", "Вопросы и уточнения", true
	case "user_correction":
		return "User correction", "Изменение требований пользователем", true
	case "steering_reasons":
		return "Steering reasons", "Причины корректировок", true
	default:
		return "", "", false
	}
}

func lookupAnalysisSections(key string) (string, string, bool) {
	switch key {
	case "task_types":
		return "Task types", "Типы задач", true
	case "steering_by_task_type":
		return "Steering by task type", "Корректировки по типам задач", true
	case "followups":
		return "follow-ups", "последующих реплик", true
	case "nothing_to_analyze":
		return "Nothing to analyze.", "Нет данных для анализа.", true
	case "steering_cache_hits":
		return "Steering cache hits", "Корректировок из кеша", true
	case "steering_new":
		return "Steering newly evaluated", "Новых оценок корректировок", true
	case "task_type_cache_hits":
		return "Task type cache hits", "Типов задач из кеша", true
	case "task_type_new":
		return "Task types newly evaluated", "Новых типов задач", true
	case "reason_cache_hits":
		return "Reason cache hits", "Причин из кеша", true
	case "reason_new":
		return "Reasons newly evaluated", "Новых причин", true
	default:
		return "", "", false
	}
}

func lookupSemanticStatus(key string) (string, string, bool) {
	switch key {
	case "semantic_privacy":
		return "Bounded excerpts of selected task prompts, final answers, and follow-ups will be sent to the configured Judge; the persistent semantic cache stores labels, not raw conversations.", "Ограниченные по размеру фрагменты выбранных постановок задач, финальных ответов и последующих реплик будут отправлены настроенному оценщику; постоянный семантический кеш хранит метки, а не исходные диалоги.", true
	case "semantic_progress":
		return "Semantic analysis", "Семантический анализ", true
	case "semantic_workers":
		return "workers", "воркеры", true
	case "semantic_cache_hits_short":
		return "cache hits", "из кеша", true
	case "semantic_elapsed":
		return "elapsed", "время", true
	case "semantic_cache_hits":
		return "Semantic cache hits", "Семантических результатов из кеша", true
	case "semantic_new":
		return "Semantic records newly evaluated", "Новых семантических оценок", true
	case "semantic_methodology":
		return "Methodology", "Методология", true
	case "semantic_model":
		return "Model / effort", "Модель / глубина", true
	case "semantic_prompt_version":
		return "Prompt version", "Версия промпта", true
	case "semantic_schema_version":
		return "Schema version", "Версия схемы", true
	default:
		return "", "", false
	}
}

func lookupSemanticTiming(key string) (string, string, bool) {
	switch key {
	case "timing_discovery":
		return "session discovery", "поиск сессий", true
	case "timing_parsing":
		return "local parsing", "локальный разбор", true
	case "timing_cache_lookup":
		return "cache lookup", "поиск в кеше", true
	case "timing_judge":
		return "semantic Judge", "семантический оценщик", true
	case "timing_conversion":
		return "result conversion", "преобразование результатов", true
	case "timing_aggregation":
		return "aggregation", "агрегация", true
	case "timing_report":
		return "final report", "итоговый отчёт", true
	default:
		return "", "", false
	}
}

func lookupEffectivenessCohorts(key string) (string, string, bool) {
	switch key {
	case "effectiveness":
		return "Model and reasoning effectiveness", "Эффективность модели и глубины рассуждений", true
	case "effectiveness_insufficient":
		return "Insufficient cohort evidence for a comparison (minimum 10 samples per cohort).", "Недостаточно данных для сравнения когорт (минимум 10 наблюдений в каждой).", true
	case "effectiveness_caveat":
		return "These are associations, not causal effects; harder tasks may be routed to higher reasoning levels.", "Это взаимосвязь, а не причинный эффект: более сложные задачи могут направляться на более глубокие уровни рассуждений.", true
	case "subagent_effectiveness":
		return "Subagent effectiveness", "Эффективность субагентов", true
	case "subagent_selection_bias":
		return "Selection bias warning: difficult tasks are more likely to use subagents, so this comparison does not establish that subagents help or hurt.", "Предупреждение о смещении выборки: сложные задачи чаще используют субагентов, поэтому сравнение не доказывает, что субагенты помогают или вредят.", true
	case "with_subagents":
		return "With subagents", "С субагентами", true
	case "without_subagents":
		return "Without subagents", "Без субагентов", true
	case "samples":
		return "samples", "наблюдений", true
	case "steering_rate":
		return "steering", "корректировки", true
	case "average_tokens":
		return "avg tokens", "средние токены", true
	case "average_seconds":
		return "avg seconds", "средние секунды", true
	default:
		return "", "", false
	}
}

func lookupEffectivenessInsights(key string) (string, string, bool) {
	switch key {
	case "average_tool_calls":
		return "avg tool calls", "средние вызовы инструментов", true
	case "insights_summary":
		return "Summary", "Итог", true
	case "insights_strengths":
		return "Strengths", "Сильные стороны", true
	case "insights_weaknesses":
		return "Weaknesses", "Слабые стороны", true
	case "insights_high":
		return "High-priority recommendations", "Рекомендации высокого приоритета", true
	case "insights_medium":
		return "Medium-priority recommendations", "Рекомендации среднего приоритета", true
	case "insights_low":
		return "Low-priority recommendations", "Рекомендации низкого приоритета", true
	case "insights_limited":
		return "Evidence is limited; no reliable cohort comparison is available yet.", "Данных мало: надёжное сравнение когорт пока невозможно.", true
	default:
		return "", "", false
	}
}

func lookupGoldenExport(key string) (string, string, bool) {
	switch key {
	case "golden_export_title":
		return "Golden fixture candidate exported", "Кандидат golden-фикстуры экспортирован", true
	case "golden_candidate_cases":
		return "Candidate cases", "Кандидатов", true
	case "golden_output":
		return "Output", "Файл", true
	case "golden_local_only":
		return "Only minimized, automatically redacted local data was written; no Judge was called. Review the candidate locally because redaction is not a guarantee of secrecy. Human approval is required before evaluation.", "Записаны только минимизированные локальные данные с автоматической очисткой; оценщик не вызывался. Проверьте кандидат локально: очистка не гарантирует удаления всех секретов. Перед оценкой требуется одобрение человека.", true
	case "golden_evaluate_title":
		return "Golden fixture evaluation", "Оценка golden-фикстуры", true
	case "golden_methodology_warning":
		return "Warning: fixture methodology differs from current (%s / %s / %s vs %s / %s / %s); evaluation uses the current methodology.", "Внимание: методология фикстуры отличается от текущей (%s / %s / %s вместо %s / %s / %s); используется текущая методология.", true
	default:
		return "", "", false
	}
}

func lookupGoldenFields(key string) (string, string, bool) {
	switch key {
	case "golden_field_task_type":
		return "task type", "тип задачи", true
	case "golden_field_followup":
		return "follow-up label", "метка продолжения", true
	case "golden_field_steering_reason":
		return "steering reason", "причина корректировки", true
	case "golden_field_prompt_issue":
		return "prompt issue", "проблема постановки", true
	case "golden_field_agents_rule":
		return "AGENTS.md rule", "правило AGENTS.md", true
	case "golden_field_skill_candidate":
		return "Skill candidate", "кандидат Skill", true
	case "golden_field_validation_type":
		return "validation type", "тип проверки", true
	case "golden_field_prevention":
		return "prevention mechanisms", "механизмы предотвращения", true
	default:
		return "", "", false
	}
}

func lookupGoldenMetrics(key string) (string, string, bool) {
	switch key {
	case "golden_samples":
		return "samples", "наблюдений", true
	case "golden_exact":
		return "exact matches", "точных совпадений", true
	case "golden_precision":
		return "precision", "точность", true
	case "golden_recall":
		return "recall", "полнота", true
	case "golden_f1":
		return "F1", "F1", true
	case "golden_expected":
		return "expected", "ожидалось", true
	case "golden_predicted":
		return "predicted", "получено", true
	default:
		return "", "", false
	}
}

func lookupHistorical(key string) (string, string, bool) {
	switch key {
	case "historical_guard":
		return "Historical aggregate regression guard", "Проверка исторического агрегированного эталона", true
	case "historical_pass":
		return "PASS", "ПРОЙДЕНО", true
	case "historical_warning":
		return "WARNING (semantic drift)", "ПРЕДУПРЕЖДЕНИЕ (семантический дрейф)", true
	case "historical_fail":
		return "FAIL", "ОШИБКА", true
	case "historical_deterministic_mismatch":
		return "deterministic mismatch", "расхождение детерминированных данных", true
	case "historical_semantic_warning":
		return "semantic tolerance warning", "предупреждение о допуске семантики", true
	case "historical_calibration_warning":
		return "Calibration required: historical reference uses %s; current methodology is %s. Follow-up and semantic metrics are uncalibrated.", "Требуется калибровка: исторический эталон использует %s; текущая методология — %s. Метрики продолжений и семантики не откалиброваны.", true
	default:
		return "", "", false
	}
}
