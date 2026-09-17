package sessions

import "strings"

type Followup struct {
	PreviousTurnID string
	TurnID         string
	PreviousAnswer string
	Prompt         string
}

func BuildFollowups(interactions []Interaction) []Followup {
	completed := make([]Interaction, 0, len(interactions))

	for _, interaction := range interactions {
		if interaction.Status == "complete" {
			completed = append(completed, interaction)
		}
	}

	if len(completed) < 2 {
		return nil
	}

	result := make([]Followup, 0, len(completed)-1)

	for i := 1; i < len(completed); i++ {
		previous := completed[i-1]
		current := completed[i]
		if strings.TrimSpace(current.Prompt) == "" {
			continue
		}

		result = append(result, Followup{
			PreviousTurnID: previous.TurnID,
			TurnID:         current.TurnID,
			PreviousAnswer: previous.Answer,
			Prompt:         current.Prompt,
		})
	}

	return result
}
