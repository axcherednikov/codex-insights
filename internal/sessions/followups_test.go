package sessions

import (
	"reflect"
	"testing"
)

func TestBuildFollowupsLinksAdjacentCompletedInteractions(t *testing.T) {
	interactions := []Interaction{
		{TurnID: "first", Status: "complete", Prompt: "first request", Answer: "first answer"},
		{TurnID: "second", Status: "complete", Prompt: "second request", Answer: "second answer"},
	}

	want := []Followup{{
		PreviousTurnID: "first",
		TurnID:         "second",
		PreviousAnswer: "first answer",
		Prompt:         "second request",
	}}
	if got := BuildFollowups(interactions); !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildFollowups() = %#v, want %#v", got, want)
	}
}

func TestBuildFollowupsDoesNotCrossNonCompleteInteraction(t *testing.T) {
	for _, status := range []string{"aborted", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			interactions := []Interaction{
				{TurnID: "a", Status: "complete", Prompt: "task A", Answer: "answer A"},
				{TurnID: "b", Status: status, Prompt: "task B"},
				{TurnID: "c", Status: "complete", Prompt: "task C", Answer: "answer C"},
				{TurnID: "d", Status: "complete", Prompt: "task D", Answer: "answer D"},
			}

			want := []Followup{{
				PreviousTurnID: "c",
				TurnID:         "d",
				PreviousAnswer: "answer C",
				Prompt:         "task D",
			}}
			if got := BuildFollowups(interactions); !reflect.DeepEqual(got, want) {
				t.Fatalf("BuildFollowups() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestBuildFollowupsSkipsBlankCurrentPrompt(t *testing.T) {
	followups := BuildFollowups([]Interaction{
		{TurnID: "first", Status: "complete", Prompt: "first request", Answer: "first answer"},
		{TurnID: "second", Status: "complete", Prompt: " \t\n", Answer: "second answer"},
	})
	if len(followups) != 0 {
		t.Fatalf("BuildFollowups() = %#v, want no follow-ups", followups)
	}
}
