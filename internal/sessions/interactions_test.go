package sessions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestParseInteractionsRetainsIncompleteTurnBetweenCompletedTurns(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"a"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"a","message":"prompt A"}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"a","last_agent_message":"answer A"}}
{"timestamp":"2026-09-16T10:01:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"b"}}
{"timestamp":"2026-09-16T10:01:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"b","message":"prompt B"}}
{"timestamp":"2026-09-16T10:01:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"b"}}
{"timestamp":"2026-09-16T10:02:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"c"}}
{"timestamp":"2026-09-16T10:02:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"c","message":"prompt C"}}
{"timestamp":"2026-09-16T10:02:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"c","last_agent_message":"answer C"}}
`)
	got, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Interaction{
		{TurnID: "a", StartedAt: "2026-09-16T10:00:00Z", Status: "complete", Prompt: "prompt A", Answer: "answer A"},
		{TurnID: "b", StartedAt: "2026-09-16T10:01:00Z", Status: "incomplete", Prompt: "prompt B"},
		{TurnID: "c", StartedAt: "2026-09-16T10:02:00Z", Status: "complete", Prompt: "prompt C", Answer: "answer C"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseInteractions() = %#v, want %#v", got, want)
	}
	if followups := BuildFollowups(got); len(followups) != 0 {
		t.Fatalf("followups crossed incomplete B: %#v", followups)
	}
}

func TestParseInteractionsPreservesAbortAndCutoffStates(t *testing.T) {
	path := writeInteractionFixture(t, `
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"aborted"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"aborted","message":"aborted prompt"}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"turn_aborted","turn_id":"aborted"}}
{"timestamp":"2026-09-16T10:01:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"cutoff"}}
{"timestamp":"2026-09-16T10:01:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"cutoff","message":"cutoff prompt"}}
{"timestamp":"2026-09-16T10:01:04Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"cutoff","last_agent_message":"after cutoff"}}
`)
	before, _ := time.Parse(time.RFC3339, "2026-09-16T10:01:02Z")
	got, err := ParseInteractions(path, before)
	if err != nil {
		t.Fatal(err)
	}
	want := []Interaction{
		{TurnID: "aborted", StartedAt: "2026-09-16T10:00:00Z", Status: "aborted", Prompt: "aborted prompt"},
		{TurnID: "cutoff", StartedAt: "2026-09-16T10:01:00Z", Status: "incomplete", Prompt: "cutoff prompt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseInteractions() = %#v, want %#v", got, want)
	}
}

func TestParseInteractionsSkipsMalformedAndReadsUnterminatedFinalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "malformed record\n" + `{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"last"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ParseInteractions(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Interaction{{TurnID: "last", StartedAt: "2026-09-16T10:00:00Z", Status: "incomplete"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseInteractions() = %#v, want %#v", got, want)
	}
}

func TestParseInteractionsReturnsOversizedScanError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := bytes.Repeat([]byte("x"), interactionMaxLineSize+1)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseInteractions(path, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("ParseInteractions error = %v, want scanner token size failure", err)
	}
}

func TestSessionReadersPreserveSelectedSymlinkAndMissingFileErrors(t *testing.T) {
	target := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(target, []byte(`{"timestamp":"2026-09-16T10:00:00Z","type":"session_meta","payload":{"id":"selected"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "selected.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMeta(link)
	if err != nil || meta.ID != "selected" {
		t.Fatalf("ReadMeta(selected symlink) = %#v, %v", meta, err)
	}

	_, err = ParseTurns(filepath.Join(t.TempDir(), "missing"), time.Time{})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ParseTurns missing error = %v, want os.ErrNotExist", err)
	}
}

func TestFindRolloutsWrapsTraversalFailure(t *testing.T) {
	_, err := FindRollouts(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("FindRollouts error = %v, want os.ErrNotExist", err)
	}
}

func TestIsInsightsJudgeSessionReadsMarkerRecord(t *testing.T) {
	path := writeInteractionFixture(t, `{"type":"event_msg","payload":{"type":"user_message","message":"`+JudgeMarker+`"}}`)
	found, err := IsInsightsJudgeSession(path)
	if err != nil || !found {
		t.Fatalf("IsInsightsJudgeSession() = %v, %v", found, err)
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
