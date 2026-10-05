package analyze

import (
	"errors"
	"testing"

	"github.com/axcherednikov/codex-insights/internal/sessions"
)

func TestLegacyClassifierValidationIdentitiesAndPrecedence(t *testing.T) {
	followup := sessions.Followup{PreviousTurnID: "turn-1"}
	interaction := sessions.Interaction{TurnID: "turn-1"}
	tests := []struct {
		name     string
		validate func() error
		want     error
	}{
		{"agents unexpected before unsupported", func() error {
			return validateAgentsRulesResults([]sessions.Followup{followup}, []AgentsRuleResult{{PreviousTurnID: "other", Rule: "bad"}})
		}, errUnexpectedResultID},
		{"agents duplicate before unsupported", func() error {
			return validateAgentsRulesResults([]sessions.Followup{followup}, []AgentsRuleResult{{PreviousTurnID: "turn-1", Rule: "other"}, {PreviousTurnID: "turn-1", Rule: "bad"}})
		}, errDuplicateResultID},
		{"agents unsupported", func() error {
			return validateAgentsRulesResults([]sessions.Followup{followup}, []AgentsRuleResult{{PreviousTurnID: "turn-1", Rule: "bad"}})
		}, errUnsupportedAgentsRule},
		{"agents missing", func() error {
			return validateAgentsRulesResults([]sessions.Followup{followup}, nil)
		}, errJudgeResultCount},
		{"prevention unsupported", func() error {
			_, err := normalizeAndValidatePreventionResults([]sessions.Followup{followup}, []PreventionResult{{PreviousTurnID: "turn-1", Applicable: []string{"bad"}}})
			return err
		}, errUnsupportedPrevention},
		{"prompt quality unsupported", func() error {
			return validatePromptQualityResults([]sessions.Followup{followup}, []PromptQualityResult{{PreviousTurnID: "turn-1", Issue: "bad"}})
		}, errUnsupportedPromptQuality},
		{"skill candidate unsupported", func() error {
			return validateSkillCandidatesResults([]sessions.Followup{followup}, []SkillCandidateResult{{PreviousTurnID: "turn-1", Category: "bad"}})
		}, errUnsupportedSkillCandidate},
		{"steering unsupported", func() error {
			return validateSteeringResults([]sessions.Followup{followup}, []SteeringResult{{PreviousTurnID: "turn-1", Label: "bad"}})
		}, errUnsupportedSteeringLabel},
		{"steering reason unsupported", func() error {
			return validateSteeringReasonResults([]sessions.Followup{followup}, []SteeringReasonResult{{PreviousTurnID: "turn-1", Reason: "bad"}})
		}, errUnsupportedSteeringReason},
		{"task type unsupported", func() error {
			return validateTaskTypeResults([]sessions.Interaction{interaction}, []TaskTypeResult{{TurnID: "turn-1", Type: "bad"}})
		}, errUnsupportedTaskType},
		{"validation unsupported", func() error {
			return validateValidationResults([]sessions.Followup{followup}, []ValidationResult{{PreviousTurnID: "turn-1", ValidationType: "bad"}})
		}, errUnsupportedValidationType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.validate(); !errors.Is(err, test.want) {
				t.Fatalf("validation error = %v, want identity %v", err, test.want)
			}
		})
	}
}

func TestPreventionConsistencyErrorsHaveDistinctIdentities(t *testing.T) {
	cases := []struct {
		name   string
		result PreventionResult
		want   error
	}{
		{"not preventable with mechanism", PreventionResult{PreviousTurnID: "turn-1", NotPreventable: true, Applicable: []string{"skill"}}, errNotPreventableHasMechanism},
		{"preventable without mechanism", PreventionResult{PreviousTurnID: "turn-1"}, errPreventableWithoutMechanism},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeAndValidatePreventionResults([]sessions.Followup{{PreviousTurnID: "turn-1"}}, []PreventionResult{test.result})
			if !errors.Is(err, test.want) {
				t.Fatalf("validation error = %v, want identity %v", err, test.want)
			}
		})
	}
}
