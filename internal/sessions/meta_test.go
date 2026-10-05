package sessions

import (
	"encoding/json"
	"errors"
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

func TestReadMetaWrapsPayloadDecodeFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := `{"type":"session_meta","payload":{"id":7}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadMeta(path)
	var decodeErr *json.UnmarshalTypeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("ReadMeta error = %v, want wrapped JSON type error", err)
	}
}

func TestReadMetaMissingFileRetainsNotExist(t *testing.T) {
	_, err := ReadMeta(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadMeta error = %v, want os.ErrNotExist", err)
	}
}
