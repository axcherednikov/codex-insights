package fileio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadFileSelectedPathAndSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "selected.jsonl")
	if err := os.WriteFile(target, []byte("history"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "selection")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{target, link} {
		got, err := ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if string(got) != "history" {
			t.Fatalf("ReadFile(%q) = %q, want history", path, got)
		}
	}
}

func TestReadFileMissingRetainsNotExist(t *testing.T) {
	t.Parallel()

	_, err := ReadFile(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile error = %v, want os.ErrNotExist", err)
	}
}

func TestReadJoinsCallbackAndCloseFailures(t *testing.T) {
	t.Parallel()

	callbackErr := errors.New("consumer failed")
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Read(path, func(reader io.Reader) error {
		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			return readErr
		}
		if string(data) != "data" {
			t.Fatalf("callback read %q, want data", data)
		}
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("Read error = %v, want callback cause", err)
	}
}

func TestWriteAtomicReplacesPrivately(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "new", "nested", "cache.json")
	if err := WriteAtomic(path, ".analysis-cache-", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, ".analysis-cache-", []byte("second")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("file content = %q, want second", got)
	}

	checkMode(t, path, privateFileMode)
	checkMode(t, filepath.Dir(filepath.Dir(path)), privateDirectoryMode)
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("directory contains abandoned temporary files: %v", entryNames(entries))
	}
}

func TestWriteAtomicFailurePreservesDestinationAndCleansTemporary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "destination")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "preserved")
	if err := os.WriteFile(marker, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteAtomic(path, ".cache-", []byte("new"))
	if err == nil {
		t.Fatal("WriteAtomic succeeded when destination was a directory")
	}
	got, readErr := os.ReadFile(marker)
	if readErr != nil || string(got) != "old" {
		t.Fatalf("existing destination content = %q, error = %v", got, readErr)
	}

	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("failed write left temporary files: %v", entryNames(entries))
	}
}

func TestWriteExclusiveKeepsCollisionAndPrivateMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteExclusive(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	checkMode(t, path, privateFileMode)

	err := WriteExclusive(path, []byte("second"))
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("second WriteExclusive error = %v, want os.ErrExist", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "first" {
		t.Fatalf("exclusive file content = %q, error = %v", got, readErr)
	}
}

func TestCloseWithErrorRetainsBothFailures(t *testing.T) {
	t.Parallel()

	cause := errors.New("write failed")
	file, err := os.CreateTemp(t.TempDir(), "closed-")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	joined := closeWithError("write private file", cause, file)
	if !errors.Is(joined, cause) || !errors.Is(joined, os.ErrClosed) {
		t.Fatalf("joined error = %v, want write and close failures", joined)
	}
}

func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	wantDirectory := want == privateDirectoryMode
	if info.IsDir() != wantDirectory {
		t.Fatalf("path %q directory = %v, want %v", path, info.IsDir(), wantDirectory)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %q = %04o, want %04o", path, got, want)
	}
}

func entryNames(entries []os.DirEntry) string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return strings.Join(names, ", ")
}
