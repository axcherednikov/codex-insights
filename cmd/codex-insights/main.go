package main

import (
	"fmt"
	"os"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		runQuick(nil)
		return
	}

	switch os.Args[1] {
	case "quick":
		runQuick(os.Args[2:])
	case "analyze":
		runAnalyze(os.Args[2:])
	case "version", "--version", "-version":
		fmt.Println(versionLine())
	case "golden":
		if len(os.Args) < 3 {
			printUsage()
			return
		}
		var err error
		switch os.Args[2] {
		case "export":
			err = runGoldenExport(os.Args[3:])
		case "evaluate":
			err = runGoldenEvaluate(os.Args[3:])
		default:
			printUsage()
			return
		}
		if err != nil {
			fatal(err)
		}
	default:
		printUsage()
	}
}

func versionLine() string {
	return "codex-insights " + version
}

func printUsage() {
	fmt.Println("Codex Insights")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  codex-insights quick [options]")
	fmt.Println("  codex-insights analyze [options]")
	fmt.Println("  codex-insights golden export [options]")
	fmt.Println("  codex-insights golden evaluate [options]")
	fmt.Println("  codex-insights --version")
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
