package main

import (
	"fmt"
	"sort"

	"codex-insights/internal/analyze"
	"codex-insights/internal/i18n"
)

type validationStat struct {
	Type  string
	Count int
}

func printValidation(
	tr i18n.Translator,
	results []analyze.ValidationResult,
) {
	if len(results) == 0 {
		return
	}

	counts := map[string]int{}

	for _, result := range results {
		counts[result.ValidationType]++
	}

	stats := make([]validationStat, 0, len(counts))

	for validationType, count := range counts {
		stats = append(stats, validationStat{
			Type:  validationType,
			Count: count,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Type < stats[j].Type
	})

	fmt.Println()

	if tr.Language == i18n.Russian {
		fmt.Println("Каких проверок чаще всего не хватает:")
	} else {
		fmt.Println("Most common validation gaps:")
	}

	for _, stat := range stats {
		rate := 100 *
			float64(stat.Count) /
			float64(len(results))

		fmt.Printf(
			"  %-32s %4d  %5.1f%%\n",
			tr.ValidationType(stat.Type),
			stat.Count,
			rate,
		)
	}
}
