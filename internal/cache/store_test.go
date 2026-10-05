package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveUsesAtomicValidJSONAndSupportsConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 12
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Set(string(rune('a'+i)), map[string]int{"value": i}); err != nil {
				t.Errorf("Set: %v", err)
				return
			}
			if err := store.Save(); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk diskData
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("cache is not valid JSON: %v", err)
	}
	if disk.Version != version || len(disk.Entries) != writers {
		t.Fatalf("disk = %+v", disk)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestNewWrapsVersionMismatchIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"entries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := New(path)
	if !errors.Is(err, errUnsupportedCacheVersion) || !strings.Contains(err.Error(), "unsupported cache version 99") {
		t.Fatalf("New() error = %v, want version context and typed identity", err)
	}
}

func TestSaveCreatesPrivateDirectoryAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	checkCacheMode(t, filepath.Dir(path), 0o700)
	checkCacheMode(t, path, 0o600)
}

func checkCacheMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %q = %04o, want %04o", path, got, want)
	}
}
