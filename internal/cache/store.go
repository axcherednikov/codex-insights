package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const version = 1

type diskData struct {
	Version int                        `json:"version"`
	Entries map[string]json.RawMessage `json:"entries"`
}

type Store struct {
	mu      sync.RWMutex
	saveMu  sync.Mutex
	path    string
	entries map[string]json.RawMessage
}

func NewDefault() (*Store, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("determine user cache directory: %w", err)
	}

	return New(filepath.Join(
		root,
		"codex-insights",
		"analysis-cache.json",
	))
}

func New(path string) (*Store, error) {
	store := &Store{
		path:    path,
		entries: make(map[string]json.RawMessage),
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}

	var disk diskData

	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("decode cache: %w", err)
	}

	if disk.Version != version {
		return nil, fmt.Errorf(
			"unsupported cache version %d",
			disk.Version,
		)
	}

	if disk.Entries != nil {
		store.entries = disk.Entries
	}

	return store, nil
}

func (s *Store) Get(key string, target any) bool {
	s.mu.RLock()
	data, ok := s.entries[key]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	return json.Unmarshal(data, target) == nil
}

func (s *Store) Set(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cache value: %w", err)
	}

	s.mu.Lock()
	s.entries[key] = data
	s.mu.Unlock()

	return nil
}

func (s *Store) Save() error {
	// Serialize saves as well as updates to the on-disk file. Sets that arrive
	// after this operation takes the lock are saved by a later Save call.
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(
		diskData{
			Version: version,
			Entries: s.entries,
		},
		"",
		"  ",
	)

	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	if err := os.MkdirAll(
		filepath.Dir(s.path),
		0o755,
	); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".analysis-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("create cache temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set cache temporary file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write cache: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace cache: %w", err)
	}

	return nil
}
