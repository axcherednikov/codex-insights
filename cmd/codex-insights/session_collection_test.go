package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectSessionFilesRetainsIncompleteBarrier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	data := `{"timestamp":"2026-09-16T09:59:00Z","type":"session_meta","payload":{"id":"synthetic","originator":"user","thread_source":"user"}}
{"timestamp":"2026-09-16T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"a"}}
{"timestamp":"2026-09-16T10:00:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"a","message":"prompt A"}}
{"timestamp":"2026-09-16T10:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"a","last_agent_message":"answer A"}}
{"timestamp":"2026-09-16T10:01:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"b"}}
{"timestamp":"2026-09-16T10:01:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"b","message":"unfinished prompt B"}}
{"timestamp":"2026-09-16T10:02:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"c"}}
{"timestamp":"2026-09-16T10:02:01Z","type":"event_msg","payload":{"type":"user_message","turn_id":"c","message":"prompt C"}}
{"timestamp":"2026-09-16T10:02:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"c","last_agent_message":"answer C"}}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	collected := collectSessionFiles([]string{path}, sessionWindow{Before: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)})
	if len(collected.Interactions) != 3 || collected.Interactions[1].TurnID != "b" || collected.Interactions[1].Status != "incomplete" || collected.Interactions[1].Prompt != "unfinished prompt B" {
		t.Fatalf("interactions = %#v", collected.Interactions)
	}
	if len(collected.Followups) != 0 {
		t.Fatalf("incomplete turn did not block followups: %#v", collected.Followups)
	}
}
