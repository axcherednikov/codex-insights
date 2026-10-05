package i18n

func recommendationTextTranslation(key string) (string, string, bool) {
	if english, russian, found := lookupRecommendationProgress(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupRecommendationCounts(key); found {
		return english, russian, true
	}
	if english, russian, found := lookupRecommendationHeadings(key); found {
		return english, russian, true
	}

	return "", "", false
}

func lookupRecommendationProgress(key string) (string, string, bool) {
	switch key {
	case "running_prevention":
		return "Running prevention analysis...", "Анализируем способы снижения корректировок...", true
	case "running_validation":
		return "Running validation gap analysis...", "Анализируем недостающие проверки...", true
	case "running_prompt_quality":
		return "Analyzing prompt quality...", "Анализируем качество постановок...", true
	case "running_agents_rules":
		return "Analyzing AGENTS.md recommendations...", "Формируем рекомендации для AGENTS.md...", true
	case "running_skill_candidates":
		return "Finding reusable Skill candidates...", "Ищем переиспользуемые Skill...", true
	default:
		return "", "", false
	}
}

func lookupRecommendationCounts(key string) (string, string, bool) {
	switch key {
	case "prevention_cache_hits":
		return "Prevention cache hits", "Способов предотвращения из кеша", true
	case "prevention_new":
		return "Prevention newly evaluated", "Новых оценок предотвращения", true
	case "validation_cache_hits":
		return "Validation cache hits", "Проверок из кеша", true
	case "validation_new":
		return "Validation newly evaluated", "Новых оценок проверок", true
	case "prompt_quality_cache_hits":
		return "Prompt quality cache hits", "Постановок из кеша", true
	case "prompt_quality_new":
		return "Prompt quality newly evaluated", "Новых оценок постановок", true
	case "agents_rules_cache_hits":
		return "AGENTS.md cache hits", "Правил AGENTS.md из кеша", true
	case "agents_rules_new":
		return "AGENTS.md newly evaluated", "Новых оценок правил AGENTS.md", true
	case "skill_candidates_cache_hits":
		return "Skill candidate cache hits", "Кандидатов Skill из кеша", true
	case "skill_candidates_new":
		return "Skill candidates newly evaluated", "Новых оценок кандидатов Skill", true
	default:
		return "", "", false
	}
}

func lookupRecommendationHeadings(key string) (string, string, bool) {
	switch key {
	case "prompt_quality":
		return "Prompt quality improvements", "Как улучшить постановку задачи", true
	case "agents_recommendations":
		return "Concrete AGENTS.md recommendations", "Конкретные рекомендации для AGENTS.md", true
	case "skill_candidates":
		return "Reusable Skill candidates", "Переиспользуемые кандидаты Skill", true
	case "supporting_cases":
		return "supporting cases", "подтверждающих случаев", true
	case "usefulness":
		return "Usefulness", "Польза", true
	default:
		return "", "", false
	}
}

func (t Translator) PromptQualityIssue(value string) string {
	english, russian, found := promptQualityIssueTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func promptQualityIssueTranslation(key string) (string, string, bool) {
	switch key {
	case "ambiguous_request":
		return "Ambiguous request", "Неоднозначная постановка", true
	case "missing_acceptance":
		return "Missing acceptance criteria", "Не заданы критерии готовности", true
	case "missing_constraints":
		return "Missing constraints", "Не заданы ограничения", true
	case "missing_context":
		return "Missing context", "Не хватает контекста", true
	case "missing_scope":
		return "Missing scope", "Не определены границы изменений", true
	case "other":
		return "Other", "Другое", true
	case "wrong_assumption":
		return "Wrong assumption", "Ошибочное предположение", true
	default:
		return "", "", false
	}
}

func (t Translator) AgentsRule(value string) string {
	english, russian, found := agentsRuleTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func agentsRuleTranslation(key string) (string, string, bool) {
	switch key {
	case "avoid_scope_creep":
		return "Keep changes within the requested scope.", "Не выходить за заявленные границы изменений.", true
	case "avoid_unnecessary_complexity":
		return "Keep the implementation no more complex than necessary.", "Выбирать достаточное и не более сложное решение.", true
	case "follow_explicit_constraints":
		return "Treat explicit requirements and limitations as binding.", "Считать явные требования и ограничения обязательными.", true
	case "follow_requested_output_format":
		return "Deliver the requested output format or artifact.", "Выдавать результат или артефакт в запрошенном формате.", true
	case "inspect_existing_code_first":
		return "Inspect relevant code and instructions before making changes.", "Перед изменениями изучать нужный код и инструкции.", true
	case "other":
		return "No recurring project rule identified.", "Устойчивое правило проекта не выявлено.", true
	case "preserve_project_architecture":
		return "Follow the project's existing architecture and conventions.", "Следовать существующей архитектуре и соглашениям проекта.", true
	case "validate_before_completion":
		return "Run the relevant checks and verify results before completion.", "Перед завершением запускать нужные проверки и проверять результат.", true
	default:
		return "", "", false
	}
}

func (t Translator) SkillCandidate(value string) string {
	english, russian, found := skillCandidateTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func skillCandidateTranslation(key string) (string, string, bool) {
	switch key {
	case "bounded_architecture_review":
		return "Bounded architecture review", "Ограниченное ревью архитектуры", true
	case "implementation_prompt_file":
		return "Implementation prompt file", "Файл промпта для реализации", true
	case "verification_workflow":
		return "Verification workflow", "Процесс проверки результата", true
	default:
		return "", "", false
	}
}

func (t Translator) SkillCandidatePurpose(value string) string {
	english, russian, found := skillCandidatePurposeTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func skillCandidatePurposeTranslation(key string) (string, string, bool) {
	switch key {
	case "bounded_architecture_review":
		return "Review the relevant architecture within explicit boundaries before implementation.", "До реализации проверять нужную архитектуру в заданных границах.", true
	case "implementation_prompt_file":
		return "Turn a request into a self-contained execution contract for an implementation worker.", "Превращать задачу в самодостаточный контракт для исполнителя.", true
	case "verification_workflow":
		return "Run a repeatable set of checks and verify the delivered result.", "Повторяемо запускать проверки и подтверждать готовность результата.", true
	default:
		return "", "", false
	}
}

func (t Translator) SkillCandidateUsefulness(value string) string {
	english, russian, found := skillCandidateUsefulnessTranslation(value)

	return localized(t.Language, english, russian, found, value)
}

func skillCandidateUsefulnessTranslation(key string) (string, string, bool) {
	switch key {
	case "bounded_architecture_review":
		return "Reduces architecture drift and unnecessary redesign.", "Снижает риск расхождения с архитектурой и лишней переделки.", true
	case "implementation_prompt_file":
		return "Reduces omissions when handing implementation work to another agent.", "Снижает риск пропусков при передаче реализации исполнителю.", true
	case "verification_workflow":
		return "Catches incomplete or incorrect results before handoff.", "Помогает обнаружить неполный или неверный результат до передачи.", true
	default:
		return "", "", false
	}
}
