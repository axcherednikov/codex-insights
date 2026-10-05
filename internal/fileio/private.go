package fileio

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	privateDirectoryMode  os.FileMode = 0o700
	privateFileMode       os.FileMode = 0o600
	temporaryNameBytes                = 12
	temporaryNameAttempts             = 10
)

type fileError string

func (err fileError) Error() string {
	return string(err)
}

const (
	errInvalidTemporaryPrefix fileError = "temporary file prefix must be a non-empty filename fragment"
	errTemporaryNameExhausted fileError = "could not allocate a unique private temporary file"
)

// WriteAtomic replaces path with data using a private same-directory file.
// tempPrefix is retained in the temporary name so callers keep their existing
// on-disk naming convention.
func WriteAtomic(path, tempPrefix string, data []byte) error {
	root, destination, err := openDestinationRoot(path)
	if err != nil {
		return err
	}

	temporary, file, createErr := createTemporary(root, tempPrefix)
	if createErr != nil {
		return errors.Join(createErr, wrap("close destination directory", root.Close()))
	}

	writeErr := writePrivateFile(file, data, true)
	if writeErr != nil {
		removeErr := removeTemporary(root, temporary)
		closeRootErr := root.Close()

		return errors.Join(writeErr, removeErr, wrap("close destination directory", closeRootErr))
	}

	renameErr := root.Rename(temporary, destination)
	if renameErr != nil {
		removeErr := removeTemporary(root, temporary)
		closeRootErr := root.Close()

		return errors.Join(
			fmt.Errorf("replace private file: %w", renameErr),
			removeErr,
			wrap("close destination directory", closeRootErr),
		)
	}

	return wrap("close destination directory", root.Close())
}

// WriteExclusive creates path privately and fails when it already exists.
func WriteExclusive(path string, data []byte) error {
	root, name, err := openDestinationRoot(path)
	if err != nil {
		return err
	}

	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFileMode)
	if err != nil {
		return errors.Join(fmt.Errorf("create private file exclusively: %w", err), wrap("close destination directory", root.Close()))
	}

	writeErr := writePrivateFile(file, data, false)
	if writeErr != nil {
		removeErr := removeTemporary(root, name)
		closeRootErr := root.Close()

		return errors.Join(writeErr, removeErr, wrap("close destination directory", closeRootErr))
	}

	return wrap("close destination directory", root.Close())
}

func openDestinationRoot(path string) (*os.Root, string, error) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, privateDirectoryMode); err != nil {
		return nil, "", fmt.Errorf("create private parent directory: %w", err)
	}

	physicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, "", fmt.Errorf("resolve private parent directory: %w", err)
	}

	root, err := os.OpenRoot(physicalParent)
	if err != nil {
		return nil, "", fmt.Errorf("open private parent directory: %w", err)
	}

	return root, filepath.Base(path), nil
}

func createTemporary(root *os.Root, prefix string) (string, *os.File, error) {
	if prefix == "" || filepath.Base(prefix) != prefix {
		return "", nil, errInvalidTemporaryPrefix
	}

	for range temporaryNameAttempts {
		randomBytes := make([]byte, temporaryNameBytes)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", nil, fmt.Errorf("generate private temporary file name: %w", err)
		}

		name := prefix + hex.EncodeToString(randomBytes) + ".tmp"
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFileMode)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", nil, fmt.Errorf("create private temporary file: %w", err)
		}
	}

	return "", nil, errTemporaryNameExhausted
}

func writePrivateFile(file *os.File, data []byte, syncFile bool) error {
	if err := file.Chmod(privateFileMode); err != nil {
		return closeWithError("set private file permissions", err, file)
	}

	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		return closeWithError("write private file", writeErr, file)
	}

	if syncFile {
		if err := file.Sync(); err != nil {
			return closeWithError("sync private file", err, file)
		}
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close private file: %w", err)
	}

	return nil
}

func closeWithError(operation string, cause error, file *os.File) error {
	return errors.Join(fmt.Errorf("%s: %w", operation, cause), wrap("close private file", file.Close()))
}

func removeTemporary(root *os.Root, name string) error {
	if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove private temporary file: %w", err)
	}

	return nil
}
