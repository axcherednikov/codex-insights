package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/sessions"
)

func runAnalyze(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}

	fs := flag.NewFlagSet("analyze", flag.ExitOnError)

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

	legacyExcludeOriginator := fs.String(
		"legacy-exclude-originator",
		"",
		"exclude historical sessions by originator",
	)

	if err := fs.Parse(args); err != nil {
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

	var (
		followups             []sessions.Followup
		userSessions          int
		excludedJudgeSessions int
	)

	for _, file := range files {
		meta, err := sessions.ReadMeta(file)
		if err != nil {
			continue
		}

		if meta.ThreadSource != "user" {
			continue
		}

		isJudge, err := sessions.IsInsightsJudgeSession(file)
		if err != nil {
			continue
		}

		if isJudge {
			excludedJudgeSessions++
			continue
		}

		if *legacyExcludeOriginator != "" &&
			meta.Originator == *legacyExcludeOriginator {
			continue
		}

		interactions, err := sessions.ParseInteractions(file, before)
		if err != nil {
			continue
		}

		filtered := filterInteractions(interactions, since, before)

		if len(filtered) == 0 {
			continue
		}

		userSessions++
		followups = append(
			followups,
			sessions.BuildFollowups(filtered)...,
		)
	}

	fmt.Println("Codex Insights Analyze")
	fmt.Println("======================")
	fmt.Printf("User sessions: %d\n", userSessions)
	fmt.Printf("Follow-up pairs: %d\n", len(followups))
	fmt.Printf(
		"Auto-excluded judge sessions: %d\n",
		excludedJudgeSessions,
	)

	if len(followups) == 0 {
		fmt.Println("Nothing to analyze.")
		return
	}

	fmt.Println()
	fmt.Println("Running steering analysis...")

	analyzer := analyze.NewSteeringAnalyzer()

	results, err := analyzer.Analyze(followups)
	if err != nil {
		fatal(err)
	}

	counts := map[string]int{}

	for _, result := range results {
		counts[result.Label]++
	}

	steering := counts["steering"]
	rate := 100 * float64(steering) / float64(len(results))

	fmt.Println()
	fmt.Println("Behavior:")
	fmt.Printf("  Analyzed:        %d\n", len(results))
	fmt.Printf("  Steering:        %d (%.1f%%)\n", steering, rate)
	fmt.Printf("  Continuation:    %d\n", counts["continuation"])
	fmt.Printf("  Questions:       %d\n", counts["question"])
	fmt.Printf("  User correction: %d\n", counts["user_correction"])
}

func filterInteractions(
	input []sessions.Interaction,
	since time.Time,
	before time.Time,
) []sessions.Interaction {
	result := make([]sessions.Interaction, 0, len(input))

	for _, interaction := range input {
		startedAt, err := time.Parse(
			time.RFC3339Nano,
			interaction.StartedAt,
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

		result = append(result, interaction)
	}

	return result
}
