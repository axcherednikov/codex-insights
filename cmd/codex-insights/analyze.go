package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/golden"
	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

const maxSemanticConcurrency = 32

type taskBehaviorStat struct {
	Type      string
	Followups int
	Steering  int
}

type namedCount struct {
	Name  string
	Count int
}

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

	lang := fs.String(
		"lang",
		"auto",
		"report language: auto, en, ru",
	)

	legacyExcludeOriginator := fs.String(
		"legacy-exclude-originator",
		"",
		"exclude historical sessions by originator",
	)

	concurrency := fs.Int(
		"concurrency",
		analyze.SemanticDefaultWorkers,
		"number of semantic Judge workers (1-32)",
	)

	cachePath := fs.String(
		"cache",
		"",
		"semantic cache path; empty uses the default",
	)

	verbose := fs.Bool(
		"verbose",
		false,
		"print timing and semantic methodology metadata",
	)

	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if err := validateSemanticConcurrency(*concurrency); err != nil {
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

	discoveryStarted := time.Now()
	files, err := sessions.FindRollouts(*sessionsPath)
	if err != nil {
		fatal(err)
	}
	discoveryDuration := time.Since(discoveryStarted)
	parsingStarted := time.Now()
	collected := collectSessionFiles(files, sessionWindow{Since: since, Before: before, LegacyExcludeOriginator: *legacyExcludeOriginator})
	allInteractions := collected.Interactions
	allTurns := collected.Turns
	followups := collected.Followups
	userSessions := collected.UserSessions
	excludedJudgeSessions := collected.ExcludedJudgeSessions
	parsingDuration := time.Since(parsingStarted)

	headerStarted := time.Now()
	printConsoleTitle(tr.T("analyze_title"), "======================")
	fmt.Printf("%s: %s\n", tr.T("user_sessions"), consoleMetric(userSessions))
	fmt.Printf("%s: %s\n", tr.T("tasks"), consoleMetric(len(allInteractions)))
	fmt.Printf("%s: %s\n", tr.T("followup_pairs"), consoleMetric(len(followups)))
	fmt.Printf(
		"%s: %s\n",
		tr.T("auto_excluded_judges"),
		consoleMetric(excludedJudgeSessions),
	)
	headerDuration := time.Since(headerStarted)

	if len(followups) == 0 {
		reportBodyStarted := time.Now()
		fmt.Println(tr.T("nothing_to_analyze"))
		printHumanInsights(tr, analyze.EffectivenessAnalysis{}, nil, nil, nil, nil, nil, nil, nil)
		printHistoricalGuard(tr, golden.HistoricalOptions{Days: *days, Before: before, LegacyExcludeOriginator: *legacyExcludeOriginator}, allInteractions, allTurns, followups, nil)
		if *verbose {
			defaults := analyze.DefaultSemanticConfig()
			printSemanticTimings(tr, analysisTimings{discovery: discoveryDuration, parsing: parsingDuration, report: finalReportDuration(headerDuration, time.Since(reportBodyStarted))}, analyze.SemanticStats{
				Methodology: defaults.MethodologyVersion, PromptVersion: defaults.PromptVersion,
				SchemaVersion: defaults.SchemaVersion, Model: defaults.Model, Effort: defaults.Effort,
			})
		}
		return
	}

	fmt.Println()
	store, err := newSemanticCache(*cachePath)
	if err != nil {
		fatal(err)
	}
	config := analyze.DefaultSemanticConfig()
	config.Cache = store
	config.Workers = *concurrency
	renderer := newSemanticProgressRenderer(tr, os.Stdout, stdoutIsTTY())
	var progressMu sync.Mutex
	var firstProgress bool
	var cacheLookupDuration time.Duration
	config.Progress = func(progress analyze.SemanticProgress) {
		progressMu.Lock()
		defer progressMu.Unlock()
		if !firstProgress {
			firstProgress = true
			cacheLookupDuration = progress.Elapsed
		}
		renderer.Update(progress)
	}
	semanticAnalysis, err := analyze.NewSemanticEngine(config).AnalyzeInteractions(allInteractions, followups)
	if err != nil {
		renderer.Finish()
		fatal(err)
	}
	renderer.Finish()
	semanticDuration := semanticAnalysis.Stats.Elapsed
	judgeDuration := semanticDuration - cacheLookupDuration
	if judgeDuration < 0 {
		judgeDuration = 0
	}

	conversionStarted := time.Now()
	converted, err := convertSemanticAnalysis(semanticAnalysis, followups)
	if err != nil {
		fatal(err)
	}
	conversionDuration := time.Since(conversionStarted)

	aggregationStarted := time.Now()
	effectiveness := analyze.AggregateEffectiveness(
		allTurns,
		converted.taskTypes,
		converted.steering,
	)
	steeringResults := converted.steering
	taskTypeResults := converted.taskTypes
	reasonResults := converted.reasons
	preventionResults := converted.prevention
	promptQualityResults := converted.promptQuality
	agentsRulesResults := converted.agentsRules
	skillCandidatesResults := converted.skillCandidates
	validationResults := converted.validation

	behaviorCounts := map[string]int{}

	for _, result := range steeringResults {
		behaviorCounts[result.Label]++
	}

	steeringCount := behaviorCounts["steering"]
	steeringRate := 100 *
		float64(steeringCount) /
		float64(len(steeringResults))
	aggregationDuration := time.Since(aggregationStarted)

	reportBodyStarted := time.Now()
	fmt.Println()
	printConsoleSection(tr.T("judge"))
	fmt.Printf("  %-36s %d\n", tr.T("semantic_cache_hits")+":", semanticAnalysis.Stats.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("semantic_new")+":", semanticAnalysis.Stats.EvaluatedRecords)
	fmt.Printf("  %-36s %s\n", tr.T("semantic_methodology")+":", semanticAnalysis.Stats.Methodology)
	fmt.Printf("  %-36s %s / %s\n", tr.T("semantic_model")+":", semanticAnalysis.Stats.Model, semanticAnalysis.Stats.Effort)

	fmt.Println()
	printConsoleSection(tr.T("behavior"))

	fmt.Printf(
		"  %-36s %d\n",
		tr.T("analyzed")+":",
		len(steeringResults),
	)
	fmt.Printf(
		"  %-36s %d (%.1f%%)\n",
		tr.T("steering")+":",
		steeringCount,
		steeringRate,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("continuation")+":",
		behaviorCounts["continuation"],
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("questions")+":",
		behaviorCounts["question"],
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("user_correction")+":",
		behaviorCounts["user_correction"],
	)

	printSteeringReasons(tr, reasonResults)
	printPrevention(tr, preventionResults)
	printPromptQuality(tr, promptQualityResults)
	printAgentsRecommendations(tr, agentsRulesResults)
	printSkillCandidates(tr, skillCandidatesResults)
	printValidation(tr, validationResults)
	printTaskTypes(tr, taskTypeResults)

	printSteeringByTaskType(
		tr,
		followups,
		steeringResults,
		taskTypeResults,
	)

	printEffectiveness(tr, effectiveness)
	printHumanInsights(
		tr,
		effectiveness,
		steeringResults,
		reasonResults,
		preventionResults,
		promptQualityResults,
		agentsRulesResults,
		skillCandidatesResults,
		validationResults,
	)
	printHistoricalGuard(tr, golden.HistoricalOptions{Days: *days, Before: before, LegacyExcludeOriginator: *legacyExcludeOriginator}, allInteractions, allTurns, followups, semanticAnalysis.Results)
	if *verbose {
		printSemanticTimings(tr, analysisTimings{
			discovery: discoveryDuration, parsing: parsingDuration,
			cacheLookup: cacheLookupDuration, judge: judgeDuration,
			aggregation: aggregationDuration, conversion: conversionDuration,
			report: finalReportDuration(headerDuration, time.Since(reportBodyStarted)),
		}, semanticAnalysis.Stats)
	}
}

func printSteeringReasons(
	tr i18n.Translator,
	results []analyze.SteeringReasonResult,
) {
	if len(results) == 0 {
		return
	}

	counts := map[string]int{}

	for _, result := range results {
		counts[result.Reason]++
	}

	stats := make([]namedCount, 0, len(counts))

	for name, count := range counts {
		stats = append(stats, namedCount{
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

	fmt.Println()
	printConsoleSection(tr.T("steering_reasons"))

	for _, stat := range stats {
		rate := 100 *
			float64(stat.Count) /
			float64(len(results))

		fmt.Printf(
			"  %-34s %4d  %5.1f%%\n",
			tr.SteeringReason(stat.Name),
			stat.Count,
			rate,
		)
	}
}

func printTaskTypes(
	tr i18n.Translator,
	results []analyze.TaskTypeResult,
) {
	counts := map[string]int{}

	for _, result := range results {
		counts[result.Type]++
	}

	stats := make([]taskBehaviorStat, 0, len(counts))

	for taskType, count := range counts {
		stats = append(stats, taskBehaviorStat{
			Type:      taskType,
			Followups: count,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Followups != stats[j].Followups {
			return stats[i].Followups > stats[j].Followups
		}
		return stats[i].Type < stats[j].Type
	})

	fmt.Println()
	printConsoleSection(tr.T("task_types"))

	for _, stat := range stats {
		fmt.Printf(
			"  %-28s %d\n",
			tr.TaskType(stat.Type),
			stat.Followups,
		)
	}
}

func printSteeringByTaskType(
	tr i18n.Translator,
	followups []sessions.Followup,
	steeringResults []analyze.SteeringResult,
	taskTypes []analyze.TaskTypeResult,
) {
	typeByTurn := make(map[string]string, len(taskTypes))

	for _, result := range taskTypes {
		typeByTurn[result.TurnID] = result.Type
	}

	labelByTurn := make(
		map[string]string,
		len(steeringResults),
	)

	for _, result := range steeringResults {
		labelByTurn[result.PreviousTurnID] = result.Label
	}

	statsByType := map[string]*taskBehaviorStat{}

	for _, followup := range followups {
		taskType, ok := typeByTurn[followup.PreviousTurnID]
		if !ok {
			continue
		}

		stat, ok := statsByType[taskType]
		if !ok {
			stat = &taskBehaviorStat{
				Type: taskType,
			}
			statsByType[taskType] = stat
		}

		stat.Followups++

		if labelByTurn[followup.PreviousTurnID] == "steering" {
			stat.Steering++
		}
	}

	stats := make(
		[]taskBehaviorStat,
		0,
		len(statsByType),
	)

	for _, stat := range statsByType {
		stats = append(stats, *stat)
	}

	sort.Slice(stats, func(i, j int) bool {
		leftRate := float64(stats[i].Steering) /
			float64(stats[i].Followups)

		rightRate := float64(stats[j].Steering) /
			float64(stats[j].Followups)

		if leftRate != rightRate {
			return leftRate > rightRate
		}
		return stats[i].Type < stats[j].Type
	})

	fmt.Println()
	printConsoleSection(tr.T("steering_by_task_type"))

	for _, stat := range stats {
		rate := 100 *
			float64(stat.Steering) /
			float64(stat.Followups)

		fmt.Printf(
			"  %-28s %4d %s  %4d  %5.1f%%\n",
			tr.TaskType(stat.Type),
			stat.Followups,
			tr.T("followups"),
			stat.Steering,
			rate,
		)
	}
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

func filterTurns(input []sessions.Turn, since time.Time, before time.Time) []sessions.Turn {
	result := make([]sessions.Turn, 0, len(input))
	for _, turn := range input {
		startedAt, err := time.Parse(time.RFC3339Nano, turn.StartedAt)
		if err != nil || startedAt.After(before) {
			continue
		}
		if !since.IsZero() && startedAt.Before(since) {
			continue
		}
		result = append(result, turn)
	}
	return result
}
