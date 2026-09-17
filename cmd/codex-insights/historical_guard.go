package main

import (
	"fmt"
	"strings"

	"codex-insights/internal/analyze"
	"codex-insights/internal/golden"
	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

func deterministicSnapshot(interactions []sessions.Interaction, turns []sessions.Turn, followups []sessions.Followup) golden.DeterministicSnapshot {
	result := golden.DeterministicSnapshot{Tasks: len(interactions), Followups: len(followups)}
	for _, item := range interactions {
		switch item.Status {
		case "complete":
			result.Complete++
		case "aborted":
			result.Aborted++
		case "incomplete":
			result.Incomplete++
		}
	}
	var tokens, duration, tools int64
	completedTurns := 0
	for _, turn := range turns {
		if turn.Status != "complete" {
			continue
		}
		completedTurns++
		tokens += turn.Tokens
		duration += turn.DurationMS
		tools += int64(turn.ToolCalls)
	}
	if completedTurns > 0 {
		result.AverageTokens = float64(tokens) / float64(completedTurns)
		result.AverageSeconds = float64(duration) / float64(completedTurns) / 1000
		result.AverageTools = float64(tools) / float64(completedTurns)
	}
	return result
}

func printHistoricalGuard(tr i18n.Translator, options golden.HistoricalOptions, interactions []sessions.Interaction, turns []sessions.Turn, followups []sessions.Followup, semantic []analyze.SemanticResult) {
	guard := golden.CheckHistoricalGuard(options, deterministicSnapshot(interactions, turns, followups), golden.SemanticSnapshotFromResults(followups, semantic))
	if !guard.Applicable {
		return
	}
	fmt.Println()
	printConsoleSection(tr.T("historical_guard"))
	status := tr.T("historical_pass")
	statusStyle := ansiGreen
	if guard.Fail() {
		status = tr.T("historical_fail")
		statusStyle = ansiRed
	} else if len(guard.SemanticWarnings) > 0 {
		status = tr.T("historical_warning")
		statusStyle = ansiYellow
	}
	fmt.Printf("  %s\n", ansiText(stdoutIsTTY(), statusStyle, status))
	for _, line := range guard.DeterministicMismatches {
		line = localizeHistoricalText(tr, line)
		fmt.Printf("  %s: %s\n", tr.T("historical_deterministic_mismatch"), line)
	}
	for _, line := range guard.SemanticWarnings {
		line = localizeHistoricalText(tr, line)
		fmt.Printf("  %s: %s\n", tr.T("historical_semantic_warning"), line)
	}
}

func localizeHistoricalText(tr i18n.Translator, line string) string {
	if tr.Language != i18n.Russian {
		return line
	}
	for _, replacement := range []struct{ from, to string }{
		{"average seconds", "средние секунды"}, {"average tokens", "средние токены"}, {"average tools", "средние вызовы инструментов"},
		{"incomplete", tr.T("incomplete")}, {"complete", tr.T("complete")}, {"aborted", tr.T("aborted")}, {"followups", tr.T("followup_pairs")}, {"tasks", tr.T("tasks")},
		{"overall steering", "общие " + tr.T("steering")}, {"refactor", tr.TaskType("refactor")}, {"architecture", tr.TaskType("architecture")},
		{"semantic sample count", "число семантических наблюдений"}, {"below minimum", "меньше минимума"}, {"missing", "отсутствует"}, {"has", "содержит"},
		{"observed", "наблюдаемое"}, {"reference", "эталон"}, {"tolerance", "допуск"}, {"expected", "ожидалось"}, {"predicted", "получено"}, {"samples", "наблюдений"}, {"cohort", "когорта"}, {"pp", "п.п."},
	} {
		line = strings.ReplaceAll(line, replacement.from, replacement.to)
	}
	return line
}
