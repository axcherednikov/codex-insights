package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

type modelStat struct {
	Key   string
	Count int
}

func runQuick(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}

	fs := flag.NewFlagSet("quick", flag.ExitOnError)

	sessionsPath := fs.String(
		"sessions",
		filepath.Join(home, ".codex", "sessions"),
		"path to Codex sessions",
	)

	days := fs.Int(
		"days",
		30,
		"number of days to analyze; 0 means all history",
	)

	beforeRaw := fs.String(
		"before",
		"",
		"analyze state before this RFC3339 timestamp",
	)

	lang := fs.String(
		"lang",
		"auto",
		"report language: auto, en, ru",
	)

	legacyExcludeOriginator := fs.String(
		"legacy-exclude-originator",
		"",
		"exclude historical sessions by originator; intended for legacy cleanup only",
	)

	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	tr, err := i18n.New(*lang)
	if err != nil {
		fatal(err)
	}

	before := time.Now().UTC()

	if *beforeRaw != "" {
		parsed, err := time.Parse(time.RFC3339, *beforeRaw)
		if err != nil {
			fatal(fmt.Errorf("invalid --before: %w", err))
		}

		before = parsed
	}

	var since time.Time

	if *days > 0 {
		since = before.Add(-time.Duration(*days) * 24 * time.Hour)
	}

	files, err := sessions.FindRollouts(*sessionsPath)
	if err != nil {
		fatal(err)
	}

	statusCounts := map[string]int{}
	modelCounts := map[string]int{}

	var (
		sessionCount           int
		turnCount              int
		completedCount         int
		totalTokens            int64
		totalDurationMS        int64
		totalToolCalls         int64
		readErrors             int
		excludedJudgeSessions  int
		legacyExcludedSessions int
	)

	for _, file := range files {
		meta, err := sessions.ReadMeta(file)
		if err != nil {
			readErrors++
			continue
		}

		if meta.ThreadSource != "user" {
			continue
		}

		isJudge, err := sessions.IsInsightsJudgeSession(file)
		if err != nil {
			readErrors++
			continue
		}

		if isJudge {
			excludedJudgeSessions++
			continue
		}

		if *legacyExcludeOriginator != "" &&
			meta.Originator == *legacyExcludeOriginator {
			legacyExcludedSessions++
			continue
		}

		turns, err := sessions.ParseTurns(file, before)
		if err != nil {
			readErrors++
			continue
		}

		sessionHasTurns := false

		for _, turn := range turns {
			startedAt, err := time.Parse(
				time.RFC3339Nano,
				turn.StartedAt,
			)
			if err != nil {
				continue
			}

			if startedAt.After(before) {
				continue
			}

			if !since.IsZero() && startedAt.Before(since) {
				continue
			}

			sessionHasTurns = true
			turnCount++
			statusCounts[turn.Status]++

			modelKey := turn.Model + "\t" + turn.Effort
			modelCounts[modelKey]++

			if turn.Status == "complete" {
				completedCount++
				totalTokens += turn.Tokens
				totalDurationMS += turn.DurationMS
				totalToolCalls += int64(turn.ToolCalls)
			}
		}

		if sessionHasTurns {
			sessionCount++
		}
	}

	fmt.Println(tr.T("quick_title"))
	fmt.Println("====================")

	fmt.Printf("%s: ", tr.T("period"))

	if *days == 0 {
		fmt.Print(tr.T("all_history"))
	} else {
		fmt.Printf(tr.T("last_days"), *days)
	}

	fmt.Printf(" — %s\n", before.Format(time.RFC3339))
	fmt.Printf("%s: %d\n", tr.T("user_sessions"), sessionCount)
	fmt.Printf("%s: %d\n", tr.T("tasks"), turnCount)

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("status"))

	for _, status := range []string{
		"complete",
		"aborted",
		"incomplete",
	} {
		if count := statusCounts[status]; count > 0 {
			fmt.Printf(
				"  %-16s %d\n",
				tr.T(status),
				count,
			)
		}
	}

	if completedCount > 0 {
		fmt.Println()
		fmt.Printf("%s:\n", tr.T("completed_averages"))

		fmt.Printf(
			"  %-18s %.0f\n",
			tr.T("tokens")+":",
			float64(totalTokens)/float64(completedCount),
		)

		fmt.Printf(
			"  %-18s %.1f sec\n",
			tr.T("duration")+":",
			float64(totalDurationMS)/float64(completedCount)/1000,
		)

		fmt.Printf(
			"  %-18s %.1f\n",
			tr.T("tool_calls")+":",
			float64(totalToolCalls)/float64(completedCount),
		)
	}

	stats := make([]modelStat, 0, len(modelCounts))

	for key, count := range modelCounts {
		stats = append(stats, modelStat{
			Key:   key,
			Count: count,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("model_reasoning"))

	for _, stat := range stats {
		fmt.Printf("  %4d  %s\n", stat.Count, stat.Key)
	}

	fmt.Println()
	fmt.Printf(
		"%s: %d\n",
		tr.T("auto_excluded_judges"),
		excludedJudgeSessions,
	)

	if *legacyExcludeOriginator != "" {
		fmt.Printf(
			tr.T("legacy_excluded")+": %d\n",
			*legacyExcludeOriginator,
			legacyExcludedSessions,
		)
	}

	if readErrors > 0 {
		fmt.Printf(
			"%s: %d\n",
			tr.T("read_errors"),
			readErrors,
		)
	}
}
