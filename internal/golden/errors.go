package golden

type goldenError string

func (err goldenError) Error() string {
	return string(err)
}

const (
	errUnsupportedFormatVersion goldenError = "unsupported golden format version"
	errIncompleteMethodology    goldenError = "golden methodology metadata is incomplete"
	errIncompleteApproval       goldenError = "golden fixture is not approved with complete provenance"
	errApprovalTimestamp        goldenError = "invalid approval timestamp"
	errEmptyFixture             goldenError = "golden fixture has no cases"
	errInvalidCaseID            goldenError = "invalid golden case id"
	errDuplicateCaseID          goldenError = "duplicate golden case id"
	errCaseWithoutInput         goldenError = "golden case has no semantic input"
	errCaseWithoutExpected      goldenError = "golden case has no expected semantic fields"
	errEmptyExpected            goldenError = "golden case has an empty expected object"
	errInvalidTaskType          goldenError = "invalid task_type"
	errInvalidFollowupLabel     goldenError = "invalid followup_label"
	errInvalidSteeringReason    goldenError = "invalid steering_reason"
	errInvalidPromptIssue       goldenError = "invalid prompt_issue"
	errInvalidAgentsRule        goldenError = "invalid agents_rule"
	errInvalidSkillCandidate    goldenError = "invalid skill_candidate"
	errInvalidValidationType    goldenError = "invalid validation_type"
	errTaskTypeNeedsPrompt      goldenError = "task_type requires a prompt"
	errFollowupNeedsPrompt      goldenError = "follow-up and prevention expected fields require a followup_prompt"
	errInvalidMechanism         goldenError = "invalid prevention mechanism"
	errDuplicateMechanism       goldenError = "duplicate prevention mechanism"
	errSteeringReasonNeedsLabel goldenError = "steering_reason requires followup_label=steering"
	errSteeringOnlyNeedsLabel   goldenError = "steering-only expected fields require followup_label=steering"
	errMechanismRequired        goldenError = "expected field requires matching prevention mechanism"
	errTrailingJSON             goldenError = "golden fixture contains trailing JSON"
	errOutputPathRequired       goldenError = "golden output path is required"
	errInvalidConcurrency       goldenError = "invalid evaluation concurrency"
	errMetricsResultCount       goldenError = "metrics received a result count different from cases"
	errMetricsDuplicateResult   goldenError = "metrics received duplicate result id"
	errMetricsMissingResult     goldenError = "metrics missing result for case"
)
