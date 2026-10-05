package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/axcherednikov/codex-insights/internal/i18n"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

type modelStat struct {
	Key   string
	Count int
}

type quickStats struct {
	statusCounts           map[string]int
	modelCounts            map[string]int
	sessionCount           int
	turnCount              int
	completedCount         int
	totalTokens            int64
	totalDurationMS        int64
	totalToolCalls         int64
	readErrors             int
	excludedJudgeSessions  int
	legacyExcludedSessions int
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
		quickDefaultDays,
		"number of days to analyze; 0 means all history",
	)

	beforeRaw := fs.String(
		"before",
		"",
		"analyze state before this RFC3339 timestamp",
	)

	lang := fs.String(
		"lang",
		defaultReportLanguage,
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

	before, since, err := quickWindow(*beforeRaw, *days)
	if err != nil {
		fatal(err)
	}

	files, err := sessions.FindRollouts(*sessionsPath)
	if err != nil {
		fatal(err)
	}

	stats := collectQuickStats(files, since, before, *legacyExcludeOriginator)
	printQuickReport(tr, *days, before, *legacyExcludeOriginator, stats)
}

func quickWindow(beforeRaw string, days int) (time.Time, time.Time, error) {
	before := time.Now().UTC()
	if beforeRaw != "" {
		parsed, err := time.Parse(time.RFC3339, beforeRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --before: %w", err)
		}
		before = parsed
	}
	if days <= 0 {
		return before, time.Time{}, nil
	}

	return before, before.Add(-time.Duration(days) * 24 * time.Hour), nil
}

func collectQuickStats(files []string, since, before time.Time, legacyOriginator string) quickStats {
	stats := quickStats{statusCounts: make(map[string]int), modelCounts: make(map[string]int)}
	for _, file := range files {
		collectQuickFile(&stats, file, since, before, legacyOriginator)
	}

	return stats
}

func collectQuickFile(stats *quickStats, file string, since, before time.Time, legacyOriginator string) {
	meta, err := sessions.ReadMeta(file)
	if err != nil {
		stats.readErrors++

		return
	}
	if meta.ThreadSource != "user" {
		return
	}
	isJudge, err := sessions.IsInsightsJudgeSession(file)
	if err != nil {
		stats.readErrors++

		return
	}
	if isJudge {
		if timestampInWindow(meta.StartedAt, since, before) {
			stats.excludedJudgeSessions++
		}

		return
	}
	if legacyOriginator != "" && meta.Originator == legacyOriginator {
		stats.legacyExcludedSessions++

		return
	}
	turns, err := sessions.ParseTurns(file, before)
	if err != nil {
		stats.readErrors++

		return
	}
	hasTurns := false
	for _, turn := range turns {
		if addQuickTurn(stats, turn, since, before) {
			hasTurns = true
		}
	}
	if hasTurns {
		stats.sessionCount++
	}
}

func addQuickTurn(stats *quickStats, turn sessions.Turn, since, before time.Time) bool {
	startedAt, err := time.Parse(time.RFC3339Nano, turn.StartedAt)
	if err != nil || startedAt.After(before) || (!since.IsZero() && startedAt.Before(since)) {
		return false
	}
	stats.turnCount++
	stats.statusCounts[turn.Status]++
	stats.modelCounts[turn.Model+"\t"+turn.Effort]++
	if turn.Status == "complete" {
		stats.completedCount++
		stats.totalTokens += turn.Tokens
		stats.totalDurationMS += turn.DurationMS
		stats.totalToolCalls += int64(turn.ToolCalls)
	}

	return true
}

func printQuickReport(tr i18n.Translator, days int, before time.Time, legacyOriginator string, stats quickStats) {

	fmt.Println(tr.T("quick_title"))
	fmt.Println("====================")

	fmt.Printf("%s: ", tr.T("period"))

	if days == 0 {
		fmt.Print(tr.T("all_history"))
	} else {
		fmt.Printf(tr.T("last_days"), days)
	}

	fmt.Printf(" — %s\n", before.Format(time.RFC3339))
	fmt.Printf("%s: %d\n", tr.T("user_sessions"), stats.sessionCount)
	fmt.Printf("%s: %d\n", tr.T("tasks"), stats.turnCount)

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("status"))

	for _, status := range []string{
		"complete",
		"aborted",
		"incomplete",
	} {
		if count := stats.statusCounts[status]; count > 0 {
			fmt.Printf(
				"  %-16s %d\n",
				tr.T(status),
				count,
			)
		}
	}

	if stats.completedCount > 0 {
		fmt.Println()
		fmt.Printf("%s:\n", tr.T("completed_averages"))

		fmt.Printf(
			"  %-22s %.0f\n",
			tr.T("tokens")+":",
			float64(stats.totalTokens)/float64(stats.completedCount),
		)

		fmt.Printf(
			"  %-22s %.1f %s\n",
			tr.T("duration")+":",
			float64(stats.totalDurationMS)/float64(stats.completedCount)/secondsPerMillisecond,
			tr.T("seconds_short"),
		)

		fmt.Printf(
			"  %-22s %.1f\n",
			tr.T("tool_calls")+":",
			float64(stats.totalToolCalls)/float64(stats.completedCount),
		)
	}

	models := make([]modelStat, 0, len(stats.modelCounts))

	for key, count := range stats.modelCounts {
		models = append(models, modelStat{
			Key:   key,
			Count: count,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Count > models[j].Count
	})

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("model_reasoning"))

	for _, stat := range models {
		fmt.Printf("  %4d  %s\n", stat.Count, stat.Key)
	}

	fmt.Println()
	fmt.Printf(
		"%s: %d\n",
		tr.T("auto_excluded_judges"),
		stats.excludedJudgeSessions,
	)

	if legacyOriginator != "" {
		fmt.Printf(
			tr.T("legacy_excluded")+": %d\n",
			legacyOriginator,
			stats.legacyExcludedSessions,
		)
	}

	if stats.readErrors > 0 {
		fmt.Printf(
			"%s: %d\n",
			tr.T("read_errors"),
			stats.readErrors,
		)
	}
}
