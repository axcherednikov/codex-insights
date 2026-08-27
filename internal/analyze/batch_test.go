package analyze

import (
	"errors"
	"reflect"
	"testing"
)

func TestAnalyzeBatchWithSplitRetriesValidatedSubsets(t *testing.T) {
	var sizes []int
	results, err := analyzeBatchWithSplit(
		[]int{1, 2, 3, 4, 5},
		func(items []int) ([]int, error) {
			sizes = append(sizes, len(items))
			if len(items) > 2 {
				return nil, errors.New("incomplete output")
			}
			return append([]int(nil), items...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(results, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("results = %#v", results)
	}
	if !reflect.DeepEqual(sizes, []int{5, 2, 3, 1, 2}) {
		t.Fatalf("batch sizes = %#v", sizes)
	}
}

func TestAnalyzeBatchWithSplitReturnsSingletonFailure(t *testing.T) {
	want := errors.New("bad record")
	_, err := analyzeBatchWithSplit(
		[]int{1},
		func([]int) ([]int, error) { return nil, want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
