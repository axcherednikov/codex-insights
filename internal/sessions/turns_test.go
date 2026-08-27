package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTurnsDetectsSubagentsOnlyFromActiveToolCalls(t *testing.T) {
	content := `{"timestamp":"2026-01-01T00:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"one"}}
{"timestamp":"2026-01-01T00:00:01Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"spawn_agent"}]}}
{"timestamp":"2026-01-01T00:00:02Z","type":"response_item","payload":{"type":"function_call","name":"functions.spawn_agent"}}
{"timestamp":"2026-01-01T00:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","tool_name":"other_tool"}}
{"timestamp":"2026-01-01T00:00:04Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"one","duration_ms":4000}}
{"timestamp":"2026-01-01T00:00:05Z","type":"event_msg","payload":{"type":"task_started","turn_id":"two"}}
{"timestamp":"2026-01-01T00:00:06Z","type":"response_item","payload":{"type":"function_call","name":"not_spawn_agent"}}
{"timestamp":"2026-01-01T00:00:07Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"two","duration_ms":2000}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, err := ParseTurns(path, time.Time{})
	if err != nil || len(turns) != 2 {
		t.Fatalf("ParseTurns = %#v, err = %v", turns, err)
	}
	if !turns[0].UsesSubagents || turns[0].ToolCalls != 2 {
		t.Fatalf("first turn subagent/tool counts = %v/%d", turns[0].UsesSubagents, turns[0].ToolCalls)
	}
	if turns[1].UsesSubagents || turns[1].ToolCalls != 1 {
		t.Fatalf("second turn subagent/tool counts = %v/%d", turns[1].UsesSubagents, turns[1].ToolCalls)
	}
}
