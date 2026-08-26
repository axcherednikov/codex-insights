package sessions

import (
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

	return files, err
}
