package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"codex-insights/internal/analyze"
	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

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

	var (
		allInteractions       []sessions.Interaction
		allTurns              []sessions.Turn
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
			if timestampInWindow(meta.StartedAt, since, before) {
				excludedJudgeSessions++
			}
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
		allInteractions = append(allInteractions, filtered...)

		turns, err := sessions.ParseTurns(file, before)
		if err == nil {
			allTurns = append(allTurns, filterTurns(turns, since, before)...)
		}
		followups = append(
			followups,
			sessions.BuildFollowups(filtered)...,
		)
	}

	fmt.Println(tr.T("analyze_title"))
	fmt.Println("======================")
	fmt.Printf("%s: %d\n", tr.T("user_sessions"), userSessions)
	fmt.Printf("%s: %d\n", tr.T("tasks"), len(allInteractions))
	fmt.Printf("%s: %d\n", tr.T("followup_pairs"), len(followups))
	fmt.Printf(
		"%s: %d\n",
		tr.T("auto_excluded_judges"),
		excludedJudgeSessions,
	)

	if len(followups) == 0 {
		fmt.Println(tr.T("nothing_to_analyze"))
		printHumanInsights(tr, analyze.EffectivenessAnalysis{}, nil, nil, nil, nil, nil, nil, nil)
		return
	}

	fmt.Println()
	fmt.Println(tr.T("running_steering"))

	steeringAnalyzer, err := analyze.NewSteeringAnalyzer()
	if err != nil {
		fatal(err)
	}

	steeringAnalysis, err := steeringAnalyzer.Analyze(followups)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_task_types"))

	taskTypeAnalyzer, err := analyze.NewTaskTypeAnalyzer()
	if err != nil {
		fatal(err)
	}

	taskTypeAnalysis, err := taskTypeAnalyzer.Analyze(allInteractions)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_reasons"))

	reasonAnalyzer, err := analyze.NewSteeringReasonAnalyzer()
	if err != nil {
		fatal(err)
	}

	reasonAnalysis, err := reasonAnalyzer.Analyze(
		followups,
		steeringAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_prevention"))

	preventionAnalyzer, err := analyze.NewPreventionAnalyzer()
	if err != nil {
		fatal(err)
	}

	preventionAnalysis, err := preventionAnalyzer.Analyze(
		followups,
		steeringAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_prompt_quality"))

	promptQualityAnalyzer, err := analyze.NewPromptQualityAnalyzer()
	if err != nil {
		fatal(err)
	}

	promptQualityAnalysis, err := promptQualityAnalyzer.Analyze(
		followups,
		preventionAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_agents_rules"))

	agentsRulesAnalyzer, err := analyze.NewAgentsRulesAnalyzer()
	if err != nil {
		fatal(err)
	}

	agentsRulesAnalysis, err := agentsRulesAnalyzer.Analyze(
		followups,
		preventionAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_skill_candidates"))

	skillCandidatesAnalyzer, err := analyze.NewSkillCandidatesAnalyzer()
	if err != nil {
		fatal(err)
	}

	skillCandidatesAnalysis, err := skillCandidatesAnalyzer.Analyze(
		followups,
		preventionAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	fmt.Println(tr.T("running_validation"))

	validationAnalyzer, err := analyze.NewValidationAnalyzer()
	if err != nil {
		fatal(err)
	}

	validationAnalysis, err := validationAnalyzer.Analyze(
		followups,
		preventionAnalysis.Results,
	)
	if err != nil {
		fatal(err)
	}

	behaviorCounts := map[string]int{}

	for _, result := range steeringAnalysis.Results {
		behaviorCounts[result.Label]++
	}

	steeringCount := behaviorCounts["steering"]
	steeringRate := 100 *
		float64(steeringCount) /
		float64(len(steeringAnalysis.Results))

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("judge"))

	fmt.Printf(
		"  %-36s %d\n",
		tr.T("steering_cache_hits")+":",
		steeringAnalysis.CacheHits,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("steering_new")+":",
		steeringAnalysis.Evaluated,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("task_type_cache_hits")+":",
		taskTypeAnalysis.CacheHits,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("task_type_new")+":",
		taskTypeAnalysis.Evaluated,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("reason_cache_hits")+":",
		reasonAnalysis.CacheHits,
	)
	fmt.Printf(
		"  %-36s %d\n",
		tr.T("reason_new")+":",
		reasonAnalysis.Evaluated,
	)

	fmt.Printf("  %-36s %d\n", tr.T("prevention_cache_hits"), preventionAnalysis.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("prevention_new"), preventionAnalysis.Evaluated)
	fmt.Printf("  %-36s %d\n", tr.T("prompt_quality_cache_hits"), promptQualityAnalysis.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("prompt_quality_new"), promptQualityAnalysis.Evaluated)
	fmt.Printf("  %-36s %d\n", tr.T("agents_rules_cache_hits"), agentsRulesAnalysis.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("agents_rules_new"), agentsRulesAnalysis.Evaluated)
	fmt.Printf("  %-36s %d\n", tr.T("skill_candidates_cache_hits"), skillCandidatesAnalysis.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("skill_candidates_new"), skillCandidatesAnalysis.Evaluated)
	fmt.Printf("  %-36s %d\n", tr.T("validation_cache_hits"), validationAnalysis.CacheHits)
	fmt.Printf("  %-36s %d\n", tr.T("validation_new"), validationAnalysis.Evaluated)

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("behavior"))

	fmt.Printf(
		"  %-36s %d\n",
		tr.T("analyzed")+":",
		len(steeringAnalysis.Results),
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

	printSteeringReasons(tr, reasonAnalysis.Results)
	printPrevention(tr, preventionAnalysis.Results)
	printPromptQuality(tr, promptQualityAnalysis.Results)
	printAgentsRecommendations(tr, agentsRulesAnalysis.Results)
	printSkillCandidates(tr, skillCandidatesAnalysis.Results)
	printValidation(tr, validationAnalysis.Results)
	printTaskTypes(tr, taskTypeAnalysis.Results)

	printSteeringByTaskType(
		tr,
		followups,
		steeringAnalysis.Results,
		taskTypeAnalysis.Results,
	)

	effectiveness := analyze.AggregateEffectiveness(
		allTurns,
		taskTypeAnalysis.Results,
		steeringAnalysis.Results,
	)
	printEffectiveness(tr, effectiveness)
	printHumanInsights(
		tr,
		effectiveness,
		steeringAnalysis.Results,
		reasonAnalysis.Results,
		preventionAnalysis.Results,
		promptQualityAnalysis.Results,
		agentsRulesAnalysis.Results,
		skillCandidatesAnalysis.Results,
		validationAnalysis.Results,
	)
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
		return stats[i].Count > stats[j].Count
	})

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("steering_reasons"))

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
		return stats[i].Followups > stats[j].Followups
	})

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("task_types"))

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

		return leftRate > rightRate
	})

	fmt.Println()
	fmt.Printf("%s:\n", tr.T("steering_by_task_type"))

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
