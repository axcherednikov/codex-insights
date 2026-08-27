package analyze

import "fmt"

// analyzeBatchWithSplit keeps normal Judge calls batched, but retries an
// invalid batch as smaller independently validated subsets. A failing
// singleton is returned to the caller instead of being silently skipped.
func analyzeBatchWithSplit[Input, Output any](
	items []Input,
	run func([]Input) ([]Output, error),
) ([]Output, error) {
	results, err := run(items)
	if err == nil {
		return results, nil
	}
	if len(items) <= 1 {
		return nil, err
	}

	middle := len(items) / 2
	left, leftErr := analyzeBatchWithSplit(items[:middle], run)
	if leftErr != nil {
		return nil, fmt.Errorf("analyze split batch left: %w", leftErr)
	}
	right, rightErr := analyzeBatchWithSplit(items[middle:], run)
	if rightErr != nil {
		return nil, fmt.Errorf("analyze split batch right: %w", rightErr)
	}

	return append(left, right...), nil
}
