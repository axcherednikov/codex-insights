package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSubagentTokenUsageValidatesInclusiveTotals(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		valid      bool
		total      int64
	}{
		{"inclusive subsets", `{"input_tokens":100,"cached_input_tokens":80,"output_tokens":10,"reasoning_output_tokens":4,"total_tokens":110}`, true, 110},
		{"derive input plus output", `{"input_tokens":100,"output_tokens":10}`, true, 110},
		{"total only", `{"total_tokens":110}`, true, 110},
		{"explicit zero", `{"total_tokens":0}`, true, 0},
		{"absent", `{}`, false, 0},
		{"negative", `{"total_tokens":-1}`, false, 0},
		{"reasoning counted twice", `{"input_tokens":100,"output_tokens":10,"reasoning_output_tokens":4,"total_tokens":114}`, false, 0},
		{"cached larger than input", `{"input_tokens":100,"cached_input_tokens":101,"output_tokens":10}`, false, 0},
		{"reasoning larger than output", `{"input_tokens":100,"output_tokens":10,"reasoning_output_tokens":11}`, false, 0},
		{"individual larger than total", `{"input_tokens":120,"total_tokens":110}`, false, 0},
		{"overflow", `{"input_tokens":9223372036854775807,"output_tokens":1}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var u SubagentTokenUsage
			if err := json.Unmarshal([]byte(tc.data), &u); err != nil {
				t.Fatal(err)
			}
			got, ok := u.Validated()
			if ok != tc.valid || (ok && *got.TotalTokens != tc.total) {
				t.Fatalf("Validated()=%#v/%v", got, ok)
			}
		})
	}
}

func TestSubagentTokenUsageDeltaPreservesMissingAndRejectsResets(t *testing.T) {
	var baseline, last SubagentTokenUsage
	json.Unmarshal([]byte(`{"total_tokens":100}`), &baseline)
	json.Unmarshal([]byte(`{"total_tokens":120}`), &last)
	delta, ok := last.Delta(&baseline)
	if !ok || *delta.TotalTokens != 20 || delta.InputTokens != nil || delta.CachedInputTokens != nil {
		t.Fatalf("delta inferred missing components: %#v", delta)
	}
	if _, ok := baseline.Delta(&last); ok {
		t.Fatal("reset accepted as negative expenditure")
	}
	if _, ok := last.Delta(nil); ok {
		t.Fatal("missing baseline inferred as zero")
	}
}

func TestReadSubagentTurnsRetainsOwnershipWhenUsageOrDurationUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-synthetic.jsonl")
	content := `{"timestamp":"2026-10-01T00:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"owned"}}
{"timestamp":"2026-10-01T00:00:01Z","type":"token_usage_record","payload":{"thread_id":"child","turn_id":"owned","response_id":"r","usage":{"total_tokens":"invalid"}}}
{"timestamp":"2026-10-01T00:00:02Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"owned","duration_ms":"invalid"}}
{"timestamp":"2026-10-01T00:00:03Z","type":"event_msg","payload":{"type":"unrelated","duration_ms":"irrelevant","info":"irrelevant"}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, malformed, err := ReadSubagentTurns(path, "child", time.Time{}, true)
	if err != nil || malformed != 0 {
		t.Fatalf("parser=%v/%d", err, malformed)
	}
	turn := turns["owned"]
	if !turn.Owned || !turn.OwnershipSeen || len(turn.UsageRecords) != 1 || turn.UsageRecords[0].Usage != nil {
		t.Fatalf("usage invalidity erased ownership: %#v", turn)
	}
	if len(turn.Terminals) != 1 || !turn.Terminals[0].InvalidDuration || turn.Terminals[0].Status != "complete" {
		t.Fatalf("duration invalidity erased terminal: %#v", turn)
	}
}
