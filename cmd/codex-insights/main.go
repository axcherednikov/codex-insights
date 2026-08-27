package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "quick":
		runQuick(os.Args[2:])
	case "analyze":
		runAnalyze(os.Args[2:])
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Codex Insights")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  codex-insights quick [options]")
	fmt.Println("  codex-insights analyze [options]")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func timestampInWindow(raw string, since time.Time, before time.Time) bool {
	startedAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || startedAt.After(before) {
		return false
	}
	return since.IsZero() || !startedAt.Before(since)
}
