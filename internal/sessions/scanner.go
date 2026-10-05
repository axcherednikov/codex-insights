package sessions

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

func FindRollouts(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if matched, _ := filepath.Match("rollout-*.jsonl", d.Name()); matched {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk rollout directory %q: %w", root, err)
	}

	return files, nil
}
