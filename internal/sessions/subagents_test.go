package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
