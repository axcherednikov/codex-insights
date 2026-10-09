package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadSubagentTurnsRetainsExplicitRootIdentityAndToolIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	data := `{"timestamp":"2026-10-01T00:00:01Z","type":"event_msg","payload":{"type":"task_started","turn_id":"child-turn"}}
{"timestamp":"2026-10-01T00:00:02Z","type":"turn_context","payload":{"turn_id":"child-turn","root_turn_id":"user-turn","model":"actual","effort":"high"}}
{"timestamp":"2026-10-01T00:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"tool-1"}}
{"timestamp":"2026-10-01T00:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"tool-1"}}
{"timestamp":"2026-10-01T00:00:05Z","type":"token_usage_record","payload":{"thread_id":"child","turn_id":"child-turn","root_turn_id":"user-turn","response_id":"response","usage":{"total_tokens":10}}}
{"timestamp":"2026-10-01T00:00:06Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"after-cutoff"}}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := time.Parse(time.RFC3339, "2026-10-01T00:00:05Z")
	if err != nil {
		t.Fatal(err)
	}
	turns, malformed, err := ReadSubagentTurns(path, "child", before, true)
	if err != nil {
		t.Fatal(err)
	}
	turn := turns["child-turn"]
	if malformed != 0 || !turn.Owned || len(turn.Settings) != 1 || turn.Settings[0].RootTurnID != "user-turn" || len(turn.UsageRecords) != 1 || turn.UsageRecords[0].RootTurnID != "user-turn" || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ID != "tool-1" {
		t.Fatalf("lost root or tool identity: %#v", turn)
	}
}

func TestReadSubagentMetaRequiresLoggedThreadSpawn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	data := `{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"child","thread_source":"subagent","forked_from_id":"old","source":{"subagent":{"thread_spawn":{"id":"child","parent_thread_id":"parent","depth":2,"agent_role":"default"}}}}}`
	if err := os.WriteFile(path, []byte(data+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, ok, err := ReadSubagentMeta(path)
	if err != nil || !ok {
		t.Fatalf("ReadSubagentMeta() = %#v, %v, %v", meta, ok, err)
	}
	if meta.ID != "child" || meta.ParentID != "parent" || !meta.HasDepth || meta.Depth != 2 || meta.Role != "default" || meta.ForkedFromID != "old" {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestReadSubagentTurnsForkOwnershipIsExactAndTriggerDoesNotAuthenticate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-10-01T00:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"copied"}}` + "\n" +
		`{"timestamp":"2026-10-01T00:00:01Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"copied","trigger_turn":true}}` + "\n" +
		`{"timestamp":"2026-10-01T00:01:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"owned"}}` + "\n" +
		`{"timestamp":"2026-10-01T00:01:01Z","type":"token_usage_record","payload":{"thread_id":"child","turn_id":"owned"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, _, err := ReadSubagentTurns(path, "child", time.Time{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if turns["copied"].OwnershipSeen {
		t.Fatalf("trigger marker authenticated copied turn: %#v", turns["copied"])
	}
	if !turns["owned"].OwnershipSeen || !turns["owned"].Owned {
		t.Fatalf("exact ownership missing: %#v", turns["owned"])
	}
}

func TestReadSubagentTurnsIgnoresUnrelatedEventTypesWithoutTurnIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-10-01T00:00:00Z","type":"event_msg","payload":{"type":"token_count"}}` + "\n" +
		`{"timestamp":"2026-10-01T00:00:01Z","type":"event_msg","payload":{"type":"item_completed"}}` + "\n" +
		`{"timestamp":"2026-10-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started"}}` + "\n" +
		`not json`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, malformed, err := ReadSubagentTurns(path, "child", time.Time{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if malformed != 2 {
		t.Fatalf("malformed = %d, want malformed lifecycle and JSON records only", malformed)
	}
	if len(turns) != 0 {
		t.Fatalf("turns = %#v, want no turns", turns)
	}
}
