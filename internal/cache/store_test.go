package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
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
