// Package fileio provides private, rooted file operations for local data.
package fileio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Read calls consume with a reader rooted at the explicitly selected file's
// physical parent directory. Resolving the selection first preserves stable
// user-selected symlink paths while Root.Open prevents later path escape.
func Read(path string, consume func(io.Reader) error) error {
	physicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve selected file %q: %w", path, err)
	}

	physicalPath, err = filepath.Abs(physicalPath)
	if err != nil {
		return fmt.Errorf("make selected file path absolute: %w", err)
	}

	root, err := os.OpenRoot(filepath.Dir(physicalPath))
	if err != nil {
		return fmt.Errorf("open selected file directory: %w", err)
	}

	file, openErr := root.Open(filepath.Base(physicalPath))
	if openErr != nil {
		return errors.Join(fmt.Errorf("open selected file: %w", openErr), root.Close())
	}

	consumeErr := consume(file)
	closeFileErr := file.Close()
	closeRootErr := root.Close()

	return errors.Join(
		wrap("read selected file", consumeErr),
		wrap("close selected file", closeFileErr),
		wrap("close selected file directory", closeRootErr),
	)
}

// ReadFile reads an explicitly selected file through Read's rooted boundary.
func ReadFile(path string) ([]byte, error) {
	var data []byte
	err := Read(path, func(reader io.Reader) error {
		var readErr error
		data, readErr = io.ReadAll(reader)

		if readErr != nil {
			return fmt.Errorf("read selected file data: %w", readErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return data, nil
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", operation, err)
}
