package i18n

import (
	"errors"
	"testing"
)

type translationCase struct {
	category string
	key      string
	english  string
	russian  string
}

func TestCapturedDictionaryTranslations(t *testing.T) {
	cases := []translationCase{
		{category: "agents_rule", key: "avoid_scope_creep", english: "Keep changes within the requested scope.", russian: "Не выходить за заявленные границы изменений."},
		{category: "agents_rule", key: "avoid_unnecessary_complexity", english: "Keep the implementation no more complex than necessary.", russian: "Выбирать достаточное и не более сложное решение."},
		{category: "agents_rule", key: "follow_explicit_constraints", english: "Treat explicit requirements and limitations as binding.", russian: "Считать явные требования и ограничения обязательными."},
		{category: "agents_rule", key: "follow_requested_output_format", english: "Deliver the requested output format or artifact.", russian: "Выдавать результат или артефакт в запрошенном формате."},
		{category: "agents_rule", key: "inspect_existing_code_first", english: "Inspect relevant code and instructions before making changes.", russian: "Перед изменениями изучать нужный код и инструкции."},
		{category: "agents_rule", key: "other", english: "No recurring project rule identified.", russian: "Устойчивое правило проекта не выявлено."},
		{category: "agents_rule", key: "preserve_project_architecture", english: "Follow the project's existing architecture and conventions.", russian: "Следовать существующей архитектуре и соглашениям проекта."},
		{category: "agents_rule", key: "validate_before_completion", english: "Run the relevant checks and verify results before completion.", russian: "Перед завершением запускать нужные проверки и проверять результат."},
		{category: "followup", key: "continuation", english: "Continuation", russian: "Продолжение"},
		{category: "followup", key: "question", english: "Question", russian: "Вопрос"},
		{category: "followup", key: "steering", english: "Steering", russian: "Корректировка"},
		{category: "followup", key: "user_correction", english: "User correction", russian: "Изменение требования"},
		{category: "prevention", key: "agents_md", english: "AGENTS.md rules", russian: "Правила AGENTS.md"},
		{category: "prevention", key: "skill", english: "Skill", russian: "Skill"},
		{category: "prevention", key: "task_prompt", english: "Task prompt", russian: "Постановка задачи"},
		{category: "prevention", key: "validation", english: "Validation", russian: "Автоматическая проверка"},
		{category: "prompt_issue", key: "ambiguous_request", english: "Ambiguous request", russian: "Неоднозначная постановка"},
		{category: "prompt_issue", key: "missing_acceptance", english: "Missing acceptance criteria", russian: "Не заданы критерии готовности"},
		{category: "prompt_issue", key: "missing_constraints", english: "Missing constraints", russian: "Не заданы ограничения"},
		{category: "prompt_issue", key: "missing_context", english: "Missing context", russian: "Не хватает контекста"},
		{category: "prompt_issue", key: "missing_scope", english: "Missing scope", russian: "Не определены границы изменений"},
		{category: "prompt_issue", key: "other", english: "Other", russian: "Другое"},
		{category: "prompt_issue", key: "wrong_assumption", english: "Wrong assumption", russian: "Ошибочное предположение"},
		{category: "skill", key: "bounded_architecture_review", english: "Bounded architecture review", russian: "Ограниченное ревью архитектуры"},
		{category: "skill", key: "implementation_prompt_file", english: "Implementation prompt file", russian: "Файл промпта для реализации"},
		{category: "skill", key: "verification_workflow", english: "Verification workflow", russian: "Процесс проверки результата"},
		{category: "skill_purpose", key: "bounded_architecture_review", english: "Review the relevant architecture within explicit boundaries before implementation.", russian: "До реализации проверять нужную архитектуру в заданных границах."},
		{category: "skill_purpose", key: "implementation_prompt_file", english: "Turn a request into a self-contained execution contract for an implementation worker.", russian: "Превращать задачу в самодостаточный контракт для исполнителя."},
		{category: "skill_purpose", key: "verification_workflow", english: "Run a repeatable set of checks and verify the delivered result.", russian: "Повторяемо запускать проверки и подтверждать готовность результата."},
		{category: "skill_usefulness", key: "bounded_architecture_review", english: "Reduces architecture drift and unnecessary redesign.", russian: "Снижает риск расхождения с архитектурой и лишней переделки."},
		{category: "skill_usefulness", key: "implementation_prompt_file", english: "Reduces omissions when handing implementation work to another agent.", russian: "Снижает риск пропусков при передаче реализации исполнителю."},
		{category: "skill_usefulness", key: "verification_workflow", english: "Catches incomplete or incorrect results before handoff.", russian: "Помогает обнаружить неполный или неверный результат до передачи."},
		{category: "steering", key: "architecture_mismatch", english: "Architecture mismatch", russian: "Несоответствие архитектуре"},
		{category: "steering", key: "ignored_constraints", english: "Ignored constraints", russian: "Проигнорированы ограничения"},
		{category: "steering", key: "implementation_error", english: "Implementation error", russian: "Ошибка реализации"},
		{category: "steering", key: "insufficient_validation", english: "Insufficient validation", russian: "Недостаточная проверка результата"},
		{category: "steering", key: "misunderstood_request", english: "Misunderstood request", russian: "Неверно понята задача"},
		{category: "steering", key: "other", english: "Other", russian: "Другое"},
		{category: "steering", key: "overengineering", english: "Overengineering", russian: "Переусложнение решения"},
		{category: "steering", key: "wrong_output_format", english: "Wrong output format", russian: "Неверный формат результата"},
		{category: "task", key: "architecture", english: "Architecture", russian: "Архитектура"},
		{category: "task", key: "bugfix", english: "Bug fix", russian: "Исправление ошибок"},
		{category: "task", key: "code_review", english: "Code review", russian: "Ревью кода"},
		{category: "task", key: "devops", english: "DevOps", russian: "DevOps"},
		{category: "task", key: "documentation", english: "Documentation", russian: "Документация"},
		{category: "task", key: "feature", english: "Feature", russian: "Новая функциональность"},
		{category: "task", key: "other", english: "Other", russian: "Другое"},
		{category: "task", key: "refactor", english: "Refactoring", russian: "Рефакторинг"},
		{category: "task", key: "research", english: "Research", russian: "Исследование"},
		{category: "task", key: "tests", english: "Tests", russian: "Тесты"},
		{category: "task", key: "unknown", english: "Unknown task type", russian: "Неизвестный тип задачи"},
		{category: "translate", key: "aborted", english: "aborted", russian: "прервано"},
		{category: "translate", key: "agents_recommendations", english: "Concrete AGENTS.md recommendations", russian: "Конкретные рекомендации для AGENTS.md"},
		{category: "translate", key: "agents_rules_cache_hits", english: "AGENTS.md cache hits", russian: "Правил AGENTS.md из кеша"},
		{category: "translate", key: "agents_rules_new", english: "AGENTS.md newly evaluated", russian: "Новых оценок правил AGENTS.md"},
		{category: "translate", key: "all_history", english: "all history", russian: "вся история"},
		{category: "translate", key: "analyze_title", english: "Codex Insights Analyze", russian: "Codex Insights: глубокий анализ"},
		{category: "translate", key: "analyzed", english: "Analyzed", russian: "Проанализировано"},
		{category: "translate", key: "auto_excluded_judges", english: "Auto-excluded Codex Insights judge sessions", russian: "Автоматически исключено сессий LLM-оценщика Codex Insights"},
		{category: "translate", key: "average_seconds", english: "avg seconds", russian: "средние секунды"},
		{category: "translate", key: "average_tokens", english: "avg tokens", russian: "средние токены"},
		{category: "translate", key: "average_tool_calls", english: "avg tool calls", russian: "средние вызовы инструментов"},
		{category: "translate", key: "behavior", english: "Behavior", russian: "Взаимодействие с Codex"},
		{category: "translate", key: "codex_cli_missing", english: "Codex CLI was not found in PATH. Install it from https://developers.openai.com/codex/cli, then run \"codex\" once to sign in.", russian: "Codex CLI не найден в PATH. Установите его по инструкции https://developers.openai.com/codex/cli, затем один раз запустите \"codex\" и войдите в аккаунт."},
		{category: "translate", key: "complete", english: "complete", russian: "завершено"},
		{category: "translate", key: "completed_averages", english: "Completed task averages", russian: "Средние значения завершённых задач"},
		{category: "translate", key: "continuation", english: "Continuation", russian: "Продолжение работы"},
		{category: "translate", key: "duration", english: "Duration", russian: "Длительность"},
		{category: "translate", key: "effectiveness", english: "Model and reasoning effectiveness", russian: "Эффективность модели и глубины рассуждений"},
		{category: "translate", key: "effectiveness_caveat", english: "These are associations, not causal effects; harder tasks may be routed to higher reasoning levels.", russian: "Это взаимосвязь, а не причинный эффект: более сложные задачи могут направляться на более глубокие уровни рассуждений."},
		{category: "translate", key: "effectiveness_insufficient", english: "Insufficient cohort evidence for a comparison (minimum 10 samples per cohort).", russian: "Недостаточно данных для сравнения когорт (минимум 10 наблюдений в каждой)."},
		{category: "translate", key: "followup_pairs", english: "Follow-up pairs", russian: "Последующие реплики"},
		{category: "translate", key: "followups", english: "follow-ups", russian: "последующих реплик"},
		{category: "translate", key: "golden_candidate_cases", english: "Candidate cases", russian: "Кандидатов"},
		{category: "translate", key: "golden_evaluate_title", english: "Golden fixture evaluation", russian: "Оценка golden-фикстуры"},
		{category: "translate", key: "golden_exact", english: "exact matches", russian: "точных совпадений"},
		{category: "translate", key: "golden_expected", english: "expected", russian: "ожидалось"},
		{category: "translate", key: "golden_export_title", english: "Golden fixture candidate exported", russian: "Кандидат golden-фикстуры экспортирован"},
		{category: "translate", key: "golden_f1", english: "F1", russian: "F1"},
		{category: "translate", key: "golden_field_agents_rule", english: "AGENTS.md rule", russian: "правило AGENTS.md"},
		{category: "translate", key: "golden_field_followup", english: "follow-up label", russian: "метка продолжения"},
		{category: "translate", key: "golden_field_prevention", english: "prevention mechanisms", russian: "механизмы предотвращения"},
		{category: "translate", key: "golden_field_prompt_issue", english: "prompt issue", russian: "проблема постановки"},
		{category: "translate", key: "golden_field_skill_candidate", english: "Skill candidate", russian: "кандидат Skill"},
		{category: "translate", key: "golden_field_steering_reason", english: "steering reason", russian: "причина корректировки"},
		{category: "translate", key: "golden_field_task_type", english: "task type", russian: "тип задачи"},
		{category: "translate", key: "golden_field_validation_type", english: "validation type", russian: "тип проверки"},
		{category: "translate", key: "golden_local_only", english: "Only minimized, automatically redacted local data was written; no Judge was called. Review the candidate locally because redaction is not a guarantee of secrecy. Human approval is required before evaluation.", russian: "Записаны только минимизированные локальные данные с автоматической очисткой; оценщик не вызывался. Проверьте кандидат локально: очистка не гарантирует удаления всех секретов. Перед оценкой требуется одобрение человека."},
		{category: "translate", key: "golden_methodology_warning", english: "Warning: fixture methodology differs from current (%s / %s / %s vs %s / %s / %s); evaluation uses the current methodology.", russian: "Внимание: методология фикстуры отличается от текущей (%s / %s / %s вместо %s / %s / %s); используется текущая методология."},
		{category: "translate", key: "golden_output", english: "Output", russian: "Файл"},
		{category: "translate", key: "golden_precision", english: "precision", russian: "точность"},
		{category: "translate", key: "golden_predicted", english: "predicted", russian: "получено"},
		{category: "translate", key: "golden_recall", english: "recall", russian: "полнота"},
		{category: "translate", key: "golden_samples", english: "samples", russian: "наблюдений"},
		{category: "translate", key: "historical_calibration_warning", english: "Calibration required: historical reference uses %s; current methodology is %s. Follow-up and semantic metrics are uncalibrated.", russian: "Требуется калибровка: исторический эталон использует %s; текущая методология — %s. Метрики продолжений и семантики не откалиброваны."},
		{category: "translate", key: "historical_deterministic_mismatch", english: "deterministic mismatch", russian: "расхождение детерминированных данных"},
		{category: "translate", key: "historical_fail", english: "FAIL", russian: "ОШИБКА"},
		{category: "translate", key: "historical_guard", english: "Historical aggregate regression guard", russian: "Проверка исторического агрегированного эталона"},
		{category: "translate", key: "historical_pass", english: "PASS", russian: "ПРОЙДЕНО"},
		{category: "translate", key: "historical_semantic_warning", english: "semantic tolerance warning", russian: "предупреждение о допуске семантики"},
		{category: "translate", key: "historical_warning", english: "WARNING (semantic drift)", russian: "ПРЕДУПРЕЖДЕНИЕ (семантический дрейф)"},
		{category: "translate", key: "incomplete", english: "incomplete", russian: "не завершено"},
		{category: "translate", key: "insights_high", english: "High-priority recommendations", russian: "Рекомендации высокого приоритета"},
		{category: "translate", key: "insights_limited", english: "Evidence is limited; no reliable cohort comparison is available yet.", russian: "Данных мало: надёжное сравнение когорт пока невозможно."},
		{category: "translate", key: "insights_low", english: "Low-priority recommendations", russian: "Рекомендации низкого приоритета"},
		{category: "translate", key: "insights_medium", english: "Medium-priority recommendations", russian: "Рекомендации среднего приоритета"},
		{category: "translate", key: "insights_strengths", english: "Strengths", russian: "Сильные стороны"},
		{category: "translate", key: "insights_summary", english: "Summary", russian: "Итог"},
		{category: "translate", key: "insights_weaknesses", english: "Weaknesses", russian: "Слабые стороны"},
		{category: "translate", key: "judge", english: "LLM evaluation", russian: "LLM-оценка"},
		{category: "translate", key: "last_days", english: "last %d days", russian: "последние %d дней"},
		{category: "translate", key: "legacy_excluded", english: "Legacy-excluded %s sessions", russian: "Исключено старых сессий %s"},
		{category: "translate", key: "model_reasoning", english: "Model + reasoning effort", russian: "Модель + глубина рассуждений"},
		{category: "translate", key: "nothing_to_analyze", english: "Nothing to analyze.", russian: "Нет данных для анализа."},
		{category: "translate", key: "period", english: "Period", russian: "Период"},
		{category: "translate", key: "prevention_cache_hits", english: "Prevention cache hits", russian: "Способов предотвращения из кеша"},
		{category: "translate", key: "prevention_new", english: "Prevention newly evaluated", russian: "Новых оценок предотвращения"},
		{category: "translate", key: "prompt_quality", english: "Prompt quality improvements", russian: "Как улучшить постановку задачи"},
		{category: "translate", key: "prompt_quality_cache_hits", english: "Prompt quality cache hits", russian: "Постановок из кеша"},
		{category: "translate", key: "prompt_quality_new", english: "Prompt quality newly evaluated", russian: "Новых оценок постановок"},
		{category: "translate", key: "questions", english: "Questions", russian: "Вопросы и уточнения"},
		{category: "translate", key: "quick_title", english: "Codex Insights Quick", russian: "Codex Insights: быстрый отчёт"},
		{category: "translate", key: "read_errors", english: "Read errors", russian: "Ошибки чтения"},
		{category: "translate", key: "reason_cache_hits", english: "Reason cache hits", russian: "Причин из кеша"},
		{category: "translate", key: "reason_new", english: "Reasons newly evaluated", russian: "Новых причин"},
		{category: "translate", key: "running_agents_rules", english: "Analyzing AGENTS.md recommendations...", russian: "Формируем рекомендации для AGENTS.md..."},
		{category: "translate", key: "running_prevention", english: "Running prevention analysis...", russian: "Анализируем способы снижения корректировок..."},
		{category: "translate", key: "running_prompt_quality", english: "Analyzing prompt quality...", russian: "Анализируем качество постановок..."},
		{category: "translate", key: "running_reasons", english: "Running steering reason analysis...", russian: "Анализируем причины корректировок..."},
		{category: "translate", key: "running_skill_candidates", english: "Finding reusable Skill candidates...", russian: "Ищем переиспользуемые Skill..."},
		{category: "translate", key: "running_steering", english: "Running steering analysis...", russian: "Анализируем корректировки Codex..."},
		{category: "translate", key: "running_task_types", english: "Running task type analysis...", russian: "Классифицируем типы задач..."},
		{category: "translate", key: "running_validation", english: "Running validation gap analysis...", russian: "Анализируем недостающие проверки..."},
		{category: "translate", key: "samples", english: "samples", russian: "наблюдений"},
		{category: "translate", key: "seconds_short", english: "sec", russian: "с"},
		{category: "translate", key: "semantic_cache_hits", english: "Semantic cache hits", russian: "Семантических результатов из кеша"},
		{category: "translate", key: "semantic_cache_hits_short", english: "cache hits", russian: "из кеша"},
		{category: "translate", key: "semantic_elapsed", english: "elapsed", russian: "время"},
		{category: "translate", key: "semantic_methodology", english: "Methodology", russian: "Методология"},
		{category: "translate", key: "semantic_model", english: "Model / effort", russian: "Модель / глубина"},
		{category: "translate", key: "semantic_new", english: "Semantic records newly evaluated", russian: "Новых семантических оценок"},
		{category: "translate", key: "semantic_privacy", english: "Bounded excerpts of selected task prompts, final answers, and follow-ups will be sent to the configured Judge; the persistent semantic cache stores labels, not raw conversations.", russian: "Ограниченные по размеру фрагменты выбранных постановок задач, финальных ответов и последующих реплик будут отправлены настроенному оценщику; постоянный семантический кеш хранит метки, а не исходные диалоги."},
		{category: "translate", key: "semantic_progress", english: "Semantic analysis", russian: "Семантический анализ"},
		{category: "translate", key: "semantic_prompt_version", english: "Prompt version", russian: "Версия промпта"},
		{category: "translate", key: "semantic_schema_version", english: "Schema version", russian: "Версия схемы"},
		{category: "translate", key: "semantic_workers", english: "workers", russian: "воркеры"},
		{category: "translate", key: "skill_candidates", english: "Reusable Skill candidates", russian: "Переиспользуемые кандидаты Skill"},
		{category: "translate", key: "skill_candidates_cache_hits", english: "Skill candidate cache hits", russian: "Кандидатов Skill из кеша"},
		{category: "translate", key: "skill_candidates_new", english: "Skill candidates newly evaluated", russian: "Новых оценок кандидатов Skill"},
		{category: "translate", key: "status", english: "Status", russian: "Статус"},
		{category: "translate", key: "steering", english: "Steering", russian: "Корректировки Codex"},
		{category: "translate", key: "steering_by_task_type", english: "Steering by task type", russian: "Корректировки по типам задач"},
		{category: "translate", key: "steering_cache_hits", english: "Steering cache hits", russian: "Корректировок из кеша"},
		{category: "translate", key: "steering_new", english: "Steering newly evaluated", russian: "Новых оценок корректировок"},
		{category: "translate", key: "steering_rate", english: "steering", russian: "корректировки"},
		{category: "translate", key: "steering_reasons", english: "Steering reasons", russian: "Причины корректировок"},
		{category: "translate", key: "subagent_effectiveness", english: "Subagent effectiveness", russian: "Эффективность субагентов"},
		{category: "translate", key: "subagent_selection_bias", english: "Selection bias warning: difficult tasks are more likely to use subagents, so this comparison does not establish that subagents help or hurt.", russian: "Предупреждение о смещении выборки: сложные задачи чаще используют субагентов, поэтому сравнение не доказывает, что субагенты помогают или вредят."},
		{category: "translate", key: "supporting_cases", english: "supporting cases", russian: "подтверждающих случаев"},
		{category: "translate", key: "task_type_cache_hits", english: "Task type cache hits", russian: "Типов задач из кеша"},
		{category: "translate", key: "task_type_new", english: "Task types newly evaluated", russian: "Новых типов задач"},
		{category: "translate", key: "task_types", english: "Task types", russian: "Типы задач"},
		{category: "translate", key: "tasks", english: "Tasks", russian: "Задачи"},
		{category: "translate", key: "timing_aggregation", english: "aggregation", russian: "агрегация"},
		{category: "translate", key: "timing_cache_lookup", english: "cache lookup", russian: "поиск в кеше"},
		{category: "translate", key: "timing_conversion", english: "result conversion", russian: "преобразование результатов"},
		{category: "translate", key: "timing_discovery", english: "session discovery", russian: "поиск сессий"},
		{category: "translate", key: "timing_judge", english: "semantic Judge", russian: "семантический оценщик"},
		{category: "translate", key: "timing_parsing", english: "local parsing", russian: "локальный разбор"},
		{category: "translate", key: "timing_report", english: "final report", russian: "итоговый отчёт"},
		{category: "translate", key: "tokens", english: "Tokens", russian: "Токены"},
		{category: "translate", key: "tool_calls", english: "Tool calls", russian: "Вызовы инструментов"},
		{category: "translate", key: "usefulness", english: "Usefulness", russian: "Польза"},
		{category: "translate", key: "user_correction", english: "User correction", russian: "Изменение требований пользователем"},
		{category: "translate", key: "user_sessions", english: "User sessions", russian: "Пользовательские сессии"},
		{category: "translate", key: "validation_cache_hits", english: "Validation cache hits", russian: "Проверок из кеша"},
		{category: "translate", key: "validation_new", english: "Validation newly evaluated", russian: "Новых оценок проверок"},
		{category: "translate", key: "with_subagents", english: "With subagents", russian: "С субагентами"},
		{category: "translate", key: "without_subagents", english: "Without subagents", russian: "Без субагентов"},
		{category: "validation", key: "data_validation", english: "Data validation", russian: "Проверка данных"},
		{category: "validation", key: "deployment_check", english: "Deployment check", russian: "Проверка деплоя"},
		{category: "validation", key: "diff_review", english: "Diff review", russian: "Проверка итогового diff"},
		{category: "validation", key: "other", english: "Other", russian: "Другое"},
		{category: "validation", key: "output_validation", english: "Output validation", russian: "Проверка результата"},
		{category: "validation", key: "runtime_smoke_check", english: "Runtime smoke check", russian: "Проверка реального запуска"},
		{category: "validation", key: "static_analysis", english: "Static analysis", russian: "Статический анализ"},
		{category: "validation", key: "tests", english: "Automated tests", russian: "Автоматические тесты"},
	}

	for _, test := range cases {
		for _, language := range []struct {
			language Language
			want     string
		}{
			{language: English, want: test.english},
			{language: Russian, want: test.russian},
		} {
			translator := Translator{Language: language.language}
			if got := capturedLookup(translator, test.category, test.key); got != language.want {
				t.Errorf("%s(%q) in %q = %q, want %q", test.category, test.key, language.language, got, language.want)
			}
		}
	}
}

func capturedLookup(translator Translator, category, key string) string {
	switch category {
	case "translate":
		return translator.T(key)
	case "task":
		return translator.TaskType(key)
	case "steering":
		return translator.SteeringReason(key)
	case "prevention":
		return translator.PreventionMechanism(key)
	case "prompt_issue":
		return translator.PromptQualityIssue(key)
	case "agents_rule":
		return translator.AgentsRule(key)
	case "skill":
		return translator.SkillCandidate(key)
	case "skill_purpose":
		return translator.SkillCandidatePurpose(key)
	case "skill_usefulness":
		return translator.SkillCandidateUsefulness(key)
	case "followup":
		return translator.FollowupLabel(key)
	case "validation":
		return translator.ValidationType(key)
	default:
		return key
	}
}

func TestTranslatorFallbackAndZeroValue(t *testing.T) {
	unknown := "unrecognized_key"
	if got := (Translator{}).T(unknown); got != unknown {
		t.Fatalf("zero-value T(%q) = %q, want key", unknown, got)
	}
	if got := (Translator{}).TaskType(unknown); got != unknown {
		t.Fatalf("zero-value TaskType(%q) = %q, want key", unknown, got)
	}
	if got := (Translator{Language: Russian}).T("quick_title"); got != "Codex Insights: быстрый отчёт" {
		t.Fatalf("Russian lookup = %q", got)
	}
	if got := (Translator{Language: "invalid"}).T("quick_title"); got != "Codex Insights Quick" {
		t.Fatalf("invalid struct language lookup = %q", got)
	}
}

func TestNewLanguageValidationAndDetection(t *testing.T) {
	if _, err := New("xx"); !errors.Is(err, errUnsupportedLanguage) {
		t.Fatalf("New invalid language error = %v", err)
	}
	if _, err := New("xx"); err.Error() != `unsupported language "xx"; supported: auto, en, ru` {
		t.Fatalf("New invalid language message = %q", err)
	}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "ru_RU.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	for _, value := range []string{"", "auto"} {
		translator, err := New(value)
		if err != nil || translator.Language != Russian {
			t.Fatalf("New(%q) = (%q, %v), want Russian", value, translator.Language, err)
		}
	}
	t.Setenv("LC_ALL", "en_US.UTF-8")
	translator, err := New("auto")
	if err != nil || translator.Language != English {
		t.Fatalf("LC_ALL precedence = (%q, %v), want English", translator.Language, err)
	}
}

func TestTranslationLookupsDoNotAllocate(t *testing.T) {
	translator := Translator{Language: Russian}
	lookups := []func(){
		func() { _ = translator.T("quick_title") },
		func() { _ = translator.TaskType("feature") },
		func() { _ = translator.AgentsRule("avoid_scope_creep") },
		func() { _ = translator.ValidationType("tests") },
	}
	for index, lookup := range lookups {
		if allocations := testing.AllocsPerRun(100, lookup); allocations != 0 {
			t.Errorf("lookup %d allocated %v times", index, allocations)
		}
	}
}
