package i18n

func lookupSubagentEffectiveness(key string) (string, string, bool) {
	if english, russian, found := lookupSubagentEffectivenessNotes(key); found {
		return english, russian, true
	}
	switch key {
	case "subeffect_all":
		return "All task types", "Все типы задач", true
	case "subeffect_without":
		return "No observed subagents", "Без наблюдаемых субагентов", true
	case "subeffect_parent":
		return "Parent session averages", "Средние основной сессии", true
	case "subeffect_agents":
		return "Average subagent sums per user task (work seconds, not task elapsed time)", "Средние суммы агентов на пользовательскую задачу (рабочие секунды, не длительность задачи)", true
	case "subeffect_linkage":
		return "Agent turns: linked / unlinked or outside selected tasks / excluded", "Ходы агентов: связаны / без связи или вне выбранных задач / исключены", true
	case "subeffect_errors":
		return "Unreadable or malformed records", "Ошибки чтения или формата", true
	case "subeffect_coverage":
		return "known/missing turns", "известные/неполные ходы", true
	case "subeffect_counters":
		return "counter mismatches/unverified/invalid records", "расхождения/непроверенные счётчики/некорректные записи", true
	case "subeffect_role":
		return "Role", "Роль", true
	case "subeffect_model":
		return "Model", "Модель", true
	default:
		return "", "", false
	}
}

func lookupSubagentEffectivenessNotes(key string) (string, string, bool) {
	switch key {
	case "subeffect_population":
		return "Completed user tasks with existing follow-up labels only. Each task counts once per cohort; no missing follow-up is labelled successful.", "Только завершённые пользовательские задачи с существующей оценкой следующего сообщения. Задача учитывается один раз в когорте; отсутствие следующего сообщения не означает успех.", true
	case "subeffect_profiles":
		return "Observed subagent models and roles: cohorts overlap; task outcomes cannot be attributed to an individual agent.", "Наблюдаемые модели и роли субагентов: когорты пересекаются; результат задачи нельзя приписать отдельному агенту.", true
	case "subeffect_routing":
		return "Same task type and parent model / reasoning settings: routing strata reduce one selection difference, but do not measure task difficulty.", "Одинаковый тип задач и модель / глубина рассуждений основной сессии: такое разделение учитывает различия маршрутизации, но не измеряет сложность задач.", true
	case "subeffect_sources":
		return "Token sources are separate: unique token_usage_record requests take priority; token_count is a fallback estimate. Cached/reasoning tokens are included, never added twice. Parent and child costs are not summed. Averages use tasks with observed values; incomplete coverage means partial totals. Tools require distinct call_id; missing IDs give lower bounds.", "Источники токенов разделены: приоритет у уникальных запросов token_usage_record; token_count — резервная оценка. Кеш и reasoning уже входят в итог. Расходы основной сессии и агентов не суммируются. Средние рассчитаны по задачам с известными значениями; неполное покрытие означает частичные суммы. Инструменты считаются по уникальным call_id; пропущенные ID дают нижнюю границу.", true
	case "subeffect_difficulty":
		return "Task types are stratified; actual difficulty is unavailable and is not adjusted for. Ten samples per cohort is a reporting threshold, not statistical significance. Small or unmatched cohorts are descriptive only.", "Сравнение разделено по типам задач; фактическая сложность недоступна и не скорректирована. Десять наблюдений — порог отчёта, не статистическая значимость. Малые когорты и когорты без пары — только описание.", true
	case "subeffect_outcome":
		return "No steering does not prove success. Follow-up selection and incomplete historical linkage can bias cohorts; no-observed-agent tasks may have unrecorded agent activity.", "Отсутствие корректировки не доказывает успех. Отбор по последующим сообщениям и неполные исторические связи могут смещать выборки; у задач без наблюдаемых агентов могла быть незаписанная агентская работа.", true
	default:
		return "", "", false
	}
}
