package main

import (
	"fmt"
	"sort"

	"codex-insights/internal/analyze"
	"codex-insights/internal/i18n"
)

type preventionStat struct {
	Name  string
	Count int
}

func printPrevention(
	tr i18n.Translator,
	results []analyze.PreventionResult,
) {
	if len(results) == 0 {
		return
	}

	counts := map[string]int{}
	notPreventable := 0

	for _, result := range results {
		if result.NotPreventable {
			notPreventable++
		}

		for _, mechanism := range result.Applicable {
			counts[mechanism]++
		}
	}

	stats := make([]preventionStat, 0, len(counts))

	for name, count := range counts {
		stats = append(stats, preventionStat{
			Name:  name,
			Count: count,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Name < stats[j].Name
	})

	if tr.Language == i18n.Russian {
		fmt.Println()
		fmt.Println("Как снизить количество корректировок:")

		for _, stat := range stats {
			rate := 100 *
				float64(stat.Count) /
				float64(len(results))

			fmt.Printf(
				"  %-30s %4d  %5.1f%%\n",
				tr.PreventionMechanism(stat.Name),
				stat.Count,
				rate,
			)
		}

		if notPreventable > 0 {
			fmt.Printf(
				"  %-30s %4d  %5.1f%%\n",
				"Нет устойчивого способа",
				notPreventable,
				100*float64(notPreventable)/float64(len(results)),
			)
		}

		return
	}

	fmt.Println()
	fmt.Println("How to reduce steering:")

	for _, stat := range stats {
		rate := 100 *
			float64(stat.Count) /
			float64(len(results))

		fmt.Printf(
			"  %-30s %4d  %5.1f%%\n",
			tr.PreventionMechanism(stat.Name),
			stat.Count,
			rate,
		)
	}

	if notPreventable > 0 {
		fmt.Printf(
			"  %-30s %4d  %5.1f%%\n",
			"Not consistently preventable",
			notPreventable,
			100*float64(notPreventable)/float64(len(results)),
		)
	}
}
