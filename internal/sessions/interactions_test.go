package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseInteractionsExtractsModernUserPrompt(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"modern-turn"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"actual modern request"}],"internal_chat_message_metadata_passthrough":{"turn_id":"modern-turn","content_item_kinds":["user.text"]}}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"modern-turn","last_agent_message":"done"}}
`)

	interactions, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 1 || interactions[0].Prompt != "actual modern request" {
		t.Fatalf("ParseInteractions = %#v", interactions)
	}
}

func TestParseInteractionsConcatenatesModernUserPromptParts(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"modern-turn"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first request part; "},{"type":"input_text","text":"second request part"}],"internal_chat_message_metadata_passthrough":{"turn_id":"modern-turn","content_item_kinds":["user.text","user.text"]}}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"modern-turn","last_agent_message":"done"}}
`)

	interactions, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 1 || interactions[0].Prompt != "first request part; second request part" {
		t.Fatalf("ParseInteractions = %#v", interactions)
	}
}

func TestParseInteractionsIgnoresInjectedModernUserContext(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"modern-turn"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"injected environment context"}],"internal_chat_message_metadata_passthrough":{"turn_id":"modern-turn","content_item_kinds":["environments.environment_context"]}}}
{"timestamp":"2026-09-16T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"actual modern request"}],"internal_chat_message_metadata_passthrough":{"turn_id":"modern-turn","content_item_kinds":["user.text"]}}}
{"timestamp":"2026-09-16T10:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"modern-turn","last_agent_message":"done"}}
`)

	interactions, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 1 || interactions[0].Prompt != "actual modern request" {
		t.Fatalf("ParseInteractions = %#v", interactions)
	}
}

func TestParseInteractionsUsesCompletedModernUserMessageFallback(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"modern-turn"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"injected environment context"}],"internal_chat_message_metadata_passthrough":{"turn_id":"modern-turn","content_item_kinds":["environments.environment_context"]}}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"modern-turn","item":{"type":"UserMessage","content":[{"type":"text","text":"actual completed request"}]}}}
{"timestamp":"2026-09-16T10:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"modern-turn","last_agent_message":"done"}}
`)

	interactions, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 1 || interactions[0].Prompt != "actual completed request" {
		t.Fatalf("ParseInteractions = %#v", interactions)
	}
}

func TestParseInteractionsPreservesLegacyUserMessage(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"legacy-turn"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"legacy-turn","message":"legacy request"}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"legacy-turn","last_agent_message":"done"}}
`)

	interactions, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 1 || interactions[0].Prompt != "legacy request" {
		t.Fatalf("ParseInteractions = %#v", interactions)
	}
}

func TestBuildFollowupsSkipsIncompletePrompt(t *testing.T) {
	followups := BuildFollowups([]Interaction{
		{TurnID: "first", Status: "complete", Prompt: "first request", Answer: "first answer"},
		{TurnID: "second", Status: "complete", Prompt: "", Answer: "second answer"},
	})
	if len(followups) != 0 {
		t.Fatalf("BuildFollowups = %#v", followups)
	}
}

func writeInteractionFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
