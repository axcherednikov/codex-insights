package i18n

func (t Translator) PromptQualityIssue(value string) string {
	if t.Language == Russian {
		if translated, ok := russianPromptQualityIssues[value]; ok {
			return translated
		}
	}
	if translated, ok := englishPromptQualityIssues[value]; ok {
		return translated
	}
	return value
}

func (t Translator) AgentsRule(value string) string {
	if t.Language == Russian {
		if translated, ok := russianAgentsRules[value]; ok {
			return translated
		}
	}
	if translated, ok := englishAgentsRules[value]; ok {
		return translated
	}
	return value
}

func (t Translator) SkillCandidate(value string) string {
	if t.Language == Russian {
		if translated, ok := russianSkillCandidates[value]; ok {
			return translated
		}
	}
	if translated, ok := englishSkillCandidates[value]; ok {
		return translated
	}
	return value
}

func (t Translator) SkillCandidatePurpose(value string) string {
	if t.Language == Russian {
		if translated, ok := russianSkillPurposes[value]; ok {
			return translated
		}
	}
	if translated, ok := englishSkillPurposes[value]; ok {
		return translated
	}
	return value
}

func (t Translator) SkillCandidateUsefulness(value string) string {
	if t.Language == Russian {
		if translated, ok := russianSkillUsefulness[value]; ok {
			return translated
		}
	}
	if translated, ok := englishSkillUsefulness[value]; ok {
		return translated
	}
	return value
}

var englishPromptQualityIssues = map[string]string{
	"missing_constraints": "Missing constraints",
	"missing_context":     "Missing context",
	"ambiguous_request":   "Ambiguous request",
	"missing_acceptance":  "Missing acceptance criteria",
	"missing_scope":       "Missing scope",
	"wrong_assumption":    "Wrong assumption",
	"other":               "Other",
}

var russianPromptQualityIssues = map[string]string{
	"missing_constraints": "Не заданы ограничения",
	"missing_context":     "Не хватает контекста",
	"ambiguous_request":   "Неоднозначная постановка",
	"missing_acceptance":  "Не заданы критерии готовности",
	"missing_scope":       "Не определены границы изменений",
	"wrong_assumption":    "Ошибочное предположение",
	"other":               "Другое",
}

var englishAgentsRules = map[string]string{
	"avoid_unnecessary_complexity":   "Keep the implementation no more complex than necessary.",
	"preserve_project_architecture":  "Follow the project's existing architecture and conventions.",
	"follow_explicit_constraints":    "Treat explicit requirements and limitations as binding.",
	"avoid_scope_creep":              "Keep changes within the requested scope.",
	"inspect_existing_code_first":    "Inspect relevant code and instructions before making changes.",
	"validate_before_completion":     "Run the relevant checks and verify results before completion.",
	"follow_requested_output_format": "Deliver the requested output format or artifact.",
	"other":                          "No recurring project rule identified.",
}

var russianAgentsRules = map[string]string{
	"avoid_unnecessary_complexity":   "Выбирать достаточное и не более сложное решение.",
	"preserve_project_architecture":  "Следовать существующей архитектуре и соглашениям проекта.",
	"follow_explicit_constraints":    "Считать явные требования и ограничения обязательными.",
	"avoid_scope_creep":              "Не выходить за заявленные границы изменений.",
	"inspect_existing_code_first":    "Перед изменениями изучать нужный код и инструкции.",
	"validate_before_completion":     "Перед завершением запускать нужные проверки и проверять результат.",
	"follow_requested_output_format": "Выдавать результат или артефакт в запрошенном формате.",
	"other":                          "Устойчивое правило проекта не выявлено.",
}

var englishSkillCandidates = map[string]string{
	"implementation_prompt_file":  "Implementation prompt file",
	"bounded_architecture_review": "Bounded architecture review",
	"verification_workflow":       "Verification workflow",
}

var russianSkillCandidates = map[string]string{
	"implementation_prompt_file":  "Файл промпта для реализации",
	"bounded_architecture_review": "Ограниченное ревью архитектуры",
	"verification_workflow":       "Процесс проверки результата",
}

var englishSkillPurposes = map[string]string{
	"implementation_prompt_file":  "Turn a request into a self-contained execution contract for an implementation worker.",
	"bounded_architecture_review": "Review the relevant architecture within explicit boundaries before implementation.",
	"verification_workflow":       "Run a repeatable set of checks and verify the delivered result.",
}

var russianSkillPurposes = map[string]string{
	"implementation_prompt_file":  "Превращать задачу в самодостаточный контракт для исполнителя.",
	"bounded_architecture_review": "До реализации проверять нужную архитектуру в заданных границах.",
	"verification_workflow":       "Повторяемо запускать проверки и подтверждать готовность результата.",
}

var englishSkillUsefulness = map[string]string{
	"implementation_prompt_file":  "Reduces omissions when handing implementation work to another agent.",
	"bounded_architecture_review": "Reduces architecture drift and unnecessary redesign.",
	"verification_workflow":       "Catches incomplete or incorrect results before handoff.",
}

var russianSkillUsefulness = map[string]string{
	"implementation_prompt_file":  "Снижает риск пропусков при передаче реализации исполнителю.",
	"bounded_architecture_review": "Снижает риск расхождения с архитектурой и лишней переделки.",
	"verification_workflow":       "Помогает обнаружить неполный или неверный результат до передачи.",
}
