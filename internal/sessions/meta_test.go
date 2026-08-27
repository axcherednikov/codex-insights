package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMetaPreservesSessionTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := `{"timestamp":"2026-08-26T12:34:56.789Z","type":"session_meta","payload":{"id":"session","originator":"user","thread_source":"user"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	meta, err := ReadMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.StartedAt != "2026-08-26T12:34:56.789Z" {
		t.Fatalf("StartedAt = %q", meta.StartedAt)
	}
}
