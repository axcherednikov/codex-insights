package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"codex-insights/internal/analyze"
	analysiscache "codex-insights/internal/cache"
	"codex-insights/internal/i18n"
	"codex-insights/internal/sessions"
)

type semanticReportResults struct {
	steering        []analyze.SteeringResult
	taskTypes       []analyze.TaskTypeResult
	reasons         []analyze.SteeringReasonResult
	prevention      []analyze.PreventionResult
	promptQuality   []analyze.PromptQualityResult
	agentsRules     []analyze.AgentsRuleResult
	skillCandidates []analyze.SkillCandidateResult
	validation      []analyze.ValidationResult
}

func validateSemanticConcurrency(value int) error {
	if value < 1 || value > maxSemanticConcurrency {
		return fmt.Errorf("invalid --concurrency %d; must be between 1 and %d", value, maxSemanticConcurrency)
	}
	return nil
}

func newSemanticCache(path string) (*analysiscache.Store, error) {
	if path == "" {
		return analysiscache.NewDefault()
	}
	return analysiscache.New(path)
}

// convertSemanticAnalysis keeps the report layer's established result types
// while making the unified engine the only source of Judge labels.
func convertSemanticAnalysis(analysisResult analyze.SemanticAnalysis, followups []sessions.Followup) (semanticReportResults, error) {
	result := semanticReportResults{
		steering:  make([]analyze.SteeringResult, 0, len(followups)),
		taskTypes: make([]analyze.TaskTypeResult, 0, len(analysisResult.Results)),
	}
	wantedFollowups := make(map[string]struct{}, len(followups))
	for _, followup := range followups {
		if _, duplicate := wantedFollowups[followup.PreviousTurnID]; duplicate {
			return semanticReportResults{}, fmt.Errorf("duplicate explicit follow-up for turn %q", followup.PreviousTurnID)
		}
		wantedFollowups[followup.PreviousTurnID] = struct{}{}
	}
	semanticByTurn := make(map[string]analyze.SemanticResult, len(analysisResult.Results))
	for _, semantic := range analysisResult.Results {
		semanticByTurn[semantic.TurnID] = semantic
		if taskType, ok := semantic.ToTaskTypeResultIfAvailable(); ok {
			result.taskTypes = append(result.taskTypes, taskType)
		}
	}
	for _, followup := range followups {
		semantic, ok := semanticByTurn[followup.PreviousTurnID]
		if !ok {
			return semanticReportResults{}, fmt.Errorf("missing semantic result for explicit follow-up %q", followup.PreviousTurnID)
		}
		steering, ok := semantic.ToSteeringResult()
		if !ok {
			return semanticReportResults{}, fmt.Errorf("missing steering result for explicit follow-up %q", followup.PreviousTurnID)
		}
		result.steering = append(result.steering, steering)
		if item, ok := semantic.ToSteeringReasonResult(); ok {
			result.reasons = append(result.reasons, item)
		}
		if item, ok := semantic.ToPreventionResult(); ok {
			result.prevention = append(result.prevention, item)
		}
		if item, ok := semantic.ToPromptQualityResult(); ok {
			result.promptQuality = append(result.promptQuality, item)
		}
		if item, ok := semantic.ToAgentsRuleResult(); ok {
			result.agentsRules = append(result.agentsRules, item)
		}
		if item, ok := semantic.ToSkillCandidateResult(); ok {
			result.skillCandidates = append(result.skillCandidates, item)
		}
		if item, ok := semantic.ToValidationResult(); ok {
			result.validation = append(result.validation, item)
		}
	}
	return result, nil
}

type semanticProgressRenderer struct {
	translator        i18n.Translator
	writer            io.Writer
	tty               bool
	mu                sync.Mutex
	cold              bool
	started           bool
	nextPrint         int
	finished          bool
	lastProgress      analyze.SemanticProgress
	lastUpdate        time.Time
	spinnerFrame      int
	heartbeatInterval time.Duration
	heartbeatStop     chan struct{}
	heartbeatDone     chan struct{}
}

func newSemanticProgressRenderer(translator i18n.Translator, writer io.Writer, tty bool) *semanticProgressRenderer {
	return &semanticProgressRenderer{
		translator: translator, writer: writer, tty: tty,
		heartbeatInterval: 200 * time.Millisecond,
	}
}

func (r *semanticProgressRenderer) Update(progress analyze.SemanticProgress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	if !r.started {
		r.started = true
		r.cold = progress.CompletedRecords < progress.TotalRecords
		if !r.cold {
			return
		}
		fmt.Fprintln(r.writer, ansiText(r.tty, ansiDim, r.translator.T("semantic_privacy")))
		r.nextPrint = 0
	}
	if !r.cold || progress.TotalRecords <= 0 {
		return
	}
	r.lastProgress = progress
	r.lastUpdate = time.Now()
	r.startHeartbeatLocked()
	percent := progress.CompletedRecords * 100 / progress.TotalRecords
	if progress.CompletedRecords < progress.TotalRecords && percent < r.nextPrint {
		return
	}
	for r.nextPrint <= percent {
		r.nextPrint += 10
	}
	r.renderLocked(progress)
}

func (r *semanticProgressRenderer) startHeartbeatLocked() {
	if !r.tty || r.heartbeatStop != nil {
		return
	}
	r.heartbeatStop = make(chan struct{})
	r.heartbeatDone = make(chan struct{})
	interval := r.heartbeatInterval
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(r.heartbeatDone)
		for {
			select {
			case <-r.heartbeatStop:
				return
			case now := <-ticker.C:
				r.pulse(now)
			}
		}
	}()
}

func (r *semanticProgressRenderer) pulse(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || !r.cold || r.lastProgress.TotalRecords <= 0 ||
		r.lastProgress.CompletedRecords >= r.lastProgress.TotalRecords {
		return
	}
	progress := r.lastProgress
	if idle := now.Sub(r.lastUpdate); idle > 0 {
		progress.Elapsed += idle
	}
	r.renderLocked(progress)
}

var semanticSpinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (r *semanticProgressRenderer) renderLocked(progress analyze.SemanticProgress) {
	percent := progress.CompletedRecords * 100 / progress.TotalRecords
	line := fmt.Sprintf("%s: %d%% (%d/%d), %s=%d, %s=%d, %s=%s",
		r.translator.T("semantic_progress"), percent, progress.CompletedRecords, progress.TotalRecords,
		r.translator.T("semantic_workers"), progress.ConfiguredWorkers,
		r.translator.T("semantic_cache_hits_short"), progress.CacheHits,
		r.translator.T("semantic_elapsed"), formatDuration(progress.Elapsed),
	)
	if r.tty {
		style := ansiCyan
		if progress.CompletedRecords >= progress.TotalRecords {
			style = ansiGreen
		}
		line = ansiText(true, style, line)
		if progress.CompletedRecords < progress.TotalRecords {
			line += " " + ansiText(true, ansiYellow, semanticSpinnerFrames[r.spinnerFrame%len(semanticSpinnerFrames)])
			r.spinnerFrame++
		}
		fmt.Fprintf(r.writer, "\r\x1b[2K%s", line)
	} else {
		fmt.Fprintln(r.writer, line)
	}
}

func (r *semanticProgressRenderer) Finish() {
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	r.finished = true
	if r.heartbeatStop != nil {
		close(r.heartbeatStop)
	}
	done := r.heartbeatDone
	if r.cold && r.tty {
		fmt.Fprintln(r.writer)
	}
	r.mu.Unlock()
	if done != nil {
		<-done
	}
}

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func formatDuration(value time.Duration) string {
	if value < time.Millisecond {
		return "0ms"
	}
	return (value.Round(time.Millisecond)).String()
}

type analysisTimings struct {
	discovery   time.Duration
	parsing     time.Duration
	cacheLookup time.Duration
	judge       time.Duration
	aggregation time.Duration
	conversion  time.Duration
	report      time.Duration
}

func finalReportDuration(header, body time.Duration) time.Duration {
	return header + body
}

func printSemanticTimings(tr i18n.Translator, timings analysisTimings, stats analyze.SemanticStats) {
	fmt.Print(semanticTimingText(tr, timings, stats))
}

func semanticTimingText(tr i18n.Translator, timings analysisTimings, stats analyze.SemanticStats) string {
	return fmt.Sprintf("\n%s: %s; %s: %s; %s: %s; %s: %s; %s: %s; %s: %s; %s: %s\n",
		tr.T("timing_discovery"), formatDuration(timings.discovery),
		tr.T("timing_parsing"), formatDuration(timings.parsing),
		tr.T("timing_cache_lookup"), formatDuration(timings.cacheLookup),
		tr.T("timing_judge"), formatDuration(timings.judge),
		tr.T("timing_conversion"), formatDuration(timings.conversion),
		tr.T("timing_aggregation"), formatDuration(timings.aggregation),
		tr.T("timing_report"), formatDuration(timings.report),
	) + fmt.Sprintf("%s: %s; %s: %s; %s: %s; %s: %s\n",
		tr.T("semantic_methodology"), stats.Methodology,
		tr.T("semantic_prompt_version"), stats.PromptVersion,
		tr.T("semantic_schema_version"), stats.SchemaVersion,
		tr.T("semantic_model"), strings.TrimSpace(stats.Model+" / "+stats.Effort),
	)
}
