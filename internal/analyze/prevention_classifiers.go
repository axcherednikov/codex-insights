package analyze

import "codex-insights/internal/sessions"

type followupJudgeRecord struct {
	PreviousTurnID string `json:"previous_turn_id"`
	PreviousAnswer string `json:"previous_answer"`
	Followup       string `json:"followup"`
}

func expectedFollowupIDs(cases []sessions.Followup) map[string]struct{} {
	expected := make(map[string]struct{}, len(cases))
	for _, item := range cases {
		expected[item.PreviousTurnID] = struct{}{}
	}
	return expected
}

func preventionCases(
	followups []sessions.Followup,
	preventionResults []PreventionResult,
	mechanism string,
) []sessions.Followup {
	ids := make(map[string]struct{})
	for _, result := range preventionResults {
		if containsString(result.Applicable, mechanism) {
			ids[result.PreviousTurnID] = struct{}{}
		}
	}
	cases := make([]sessions.Followup, 0, len(ids))
	for _, followup := range followups {
		if _, ok := ids[followup.PreviousTurnID]; ok {
			cases = append(cases, followup)
		}
	}
	return cases
}

func followupClassifierSchema(field string, enum []string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"previous_turn_id": map[string]any{"type": "string"},
						field:              map[string]any{"type": "string", "enum": enum},
						"confidence":       map[string]any{"type": "number"},
					},
					"required":             []string{"previous_turn_id", field, "confidence"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"results"},
		"additionalProperties": false,
	}
}
