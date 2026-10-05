package analyze

type analyzeError string

func (err analyzeError) Error() string {
	return string(err)
}

const (
	errUnexpectedResultID          analyzeError = "judge returned unexpected turn id"
	errDuplicateResultID           analyzeError = "judge returned duplicate turn id"
	errJudgeResultCount            analyzeError = "judge returned"
	errMissingAgentsRuleResult     analyzeError = "missing AGENTS.md rule result for turn"
	errUnsupportedAgentsRule       analyzeError = "judge returned unsupported AGENTS.md rule"
	errMissingPreventionResult     analyzeError = "missing prevention result for turn"
	errUnsupportedPrevention       analyzeError = "judge returned unsupported prevention mechanism"
	errMissingPromptQualityResult  analyzeError = "missing prompt quality result for turn"
	errUnsupportedPromptQuality    analyzeError = "judge returned unsupported prompt quality issue"
	errMissingSkillCandidateResult analyzeError = "missing Skill candidate result for turn"
	errUnsupportedSkillCandidate   analyzeError = "judge returned unsupported Skill candidate category"
	errMissingSteeringResult       analyzeError = "missing steering result for turn"
	errUnsupportedSteeringLabel    analyzeError = "judge returned unsupported steering label"
	errMissingSteeringReason       analyzeError = "missing steering reason for turn"
	errUnsupportedSteeringReason   analyzeError = "judge returned unsupported steering reason"
	errMissingTaskTypeResult       analyzeError = "missing task type result for turn"
	errUnsupportedTaskType         analyzeError = "judge returned unsupported task type"
	errMissingValidationResult     analyzeError = "missing validation result for turn"
	errUnsupportedValidationType   analyzeError = "judge returned unsupported validation type"
)

type semanticTurnError int

const (
	errSemanticTaskWhenPromptMissing semanticTurnError = iota + 1
	errSemanticTaskAndConfidenceRequired
	errSemanticNotPreventableMissing
	errSemanticFollowupFieldsWithoutFollowup
	errSemanticFollowupAndConfidenceRequired
	errSemanticSteeringFieldsForOtherLabel
	errSemanticSteeringReasonAndConfidenceRequired
	errSemanticNotPreventableRequired
	errSemanticPreventionConfidenceWithoutMechanism
	errSemanticEmptyMechanismMustBeNotPreventable
	errSemanticPreventionConfidenceRequired
	errSemanticNotPreventableHasMechanisms
	errSemanticDuplicateMechanism
	errSemanticConditionalClassificationRequired
	errSemanticClassificationWithoutMechanism
)

func (semanticTurnError) Error() string {
	return "turn"
}

type semanticConditionalError string

const (
	errSemanticConditionalRequiresValue    semanticConditionalError = "classification required"
	errSemanticConditionalWithoutMechanism semanticConditionalError = "classification without mechanism"
)

func (err semanticConditionalError) Error() string {
	if err == errSemanticConditionalRequiresValue {
		return "prevention mechanism"
	}

	return "follow-on classification"
}

const (
	errSemanticBatchLimits          analyzeError = "semantic batch limits must be positive"
	errEmptySemanticTurnID          analyzeError = "semantic input has empty turn id"
	errDuplicateSemanticInputTurnID analyzeError = "duplicate semantic input turn id"
)

type semanticInputContextError int

const (
	errSemanticInputBudgetExceeded semanticInputContextError = iota + 1
	errBlankSemanticFollowupPrompt
)

func (semanticInputContextError) Error() string {
	return "semantic input turn"
}

type preventionConsistencyError int

const (
	errNotPreventableHasMechanism preventionConsistencyError = iota + 1
	errPreventableWithoutMechanism
)

func (preventionConsistencyError) Error() string {
	return "turn"
}
