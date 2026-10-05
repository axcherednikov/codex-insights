package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	analysiscache "github.com/axcherednikov/codex-insights/internal/cache"
	"github.com/axcherednikov/codex-insights/internal/i18n"
	"github.com/axcherednikov/codex-insights/internal/sessions"
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

const semanticProgressHeartbeatInterval = 200 * time.Millisecond

func validateSemanticConcurrency(value int) error {
	if value < minimumSemanticConcurrency || value > maxSemanticConcurrency {
		return cliValueRangeError{option: "--concurrency", value: value, minimum: minimumSemanticConcurrency, maximum: maxSemanticConcurrency, identity: errInvalidConcurrency}
	}

	return nil
}

func newSemanticCache(path string) (*analysiscache.Store, error) {
	if path == "" {
		store, err := analysiscache.NewDefault()
		if err != nil {
			return nil, fmt.Errorf("create default semantic cache: %w", err)
		}

		return store, nil
	}
	store, err := analysiscache.New(path)
	if err != nil {
		return nil, fmt.Errorf("create semantic cache: %w", err)
	}

	return store, nil
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
			return semanticReportResults{}, followupRecordError{identity: errDuplicateFollowup, turnID: followup.PreviousTurnID}
		}
		wantedFollowups[followup.PreviousTurnID] = struct{}{}
	}
	semanticByTurn := make(map[string]analyze.SemanticResult, len(analysisResult.Results))
	for _, semantic := range analysisResult.Results {
		semanticByTurn[semantic.TurnID] = semantic
		appendSemanticLabels(&result, semantic)
	}
	for _, followup := range followups {
		semantic, ok := semanticByTurn[followup.PreviousTurnID]
		if !ok {
			return semanticReportResults{}, followupRecordError{identity: errMissingSemanticResult, turnID: followup.PreviousTurnID}
		}
		steering, ok := semantic.ToSteeringResult()
		if !ok {
			return semanticReportResults{}, followupRecordError{identity: errMissingSteeringResult, turnID: followup.PreviousTurnID}
		}
		result.steering = append(result.steering, steering)
		appendFollowupLabels(&result, semantic)
	}

	return result, nil
}

func appendSemanticLabels(result *semanticReportResults, semantic analyze.SemanticResult) {
	if taskType, ok := semantic.ToTaskTypeResultIfAvailable(); ok {
		result.taskTypes = append(result.taskTypes, taskType)
	}
}

func appendFollowupLabels(result *semanticReportResults, semantic analyze.SemanticResult) {
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

type semanticProgressRenderer struct {
	translator        i18n.Translator
	writer            io.Writer
	tty               bool
	mu                sync.Mutex
	cold              bool
	started           bool
	nextPrint         int
	finished          bool
	writeErr          error
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
		heartbeatInterval: semanticProgressHeartbeatInterval,
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
		r.writeLocked(ansiText(r.tty, ansiDim, r.translator.T("semantic_privacy")) + "\n")
		if r.writeErr != nil {
			return
		}
		r.nextPrint = 0
	}
	if !r.cold || progress.TotalRecords <= 0 {
		return
	}
	r.lastProgress = progress
	r.lastUpdate = time.Now()
	r.startHeartbeatLocked()
	percent := progress.CompletedRecords * percentageScale / progress.TotalRecords
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

func semanticSpinnerFrame(index int) string {
	switch index % semanticSpinnerFrameCount {
	case semanticSpinnerFrameFirst:
		return "⠋"
	case semanticSpinnerFrameSecond:
		return "⠙"
	case semanticSpinnerFrameThird:
		return "⠹"
	case semanticSpinnerFrameFourth:
		return "⠸"
	case semanticSpinnerFrameFifth:
		return "⠼"
	case semanticSpinnerFrameSixth:
		return "⠴"
	case semanticSpinnerFrameSeventh:
		return "⠦"
	case semanticSpinnerFrameEighth:
		return "⠧"
	case semanticSpinnerFrameNinth:
		return "⠇"
	default:
		return "⠏"
	}
}

func (r *semanticProgressRenderer) renderLocked(progress analyze.SemanticProgress) {
	percent := progress.CompletedRecords * percentageScale / progress.TotalRecords
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
			line += " " + ansiText(true, ansiYellow, semanticSpinnerFrame(r.spinnerFrame))
			r.spinnerFrame++
		}
		r.writeLocked("\r\x1b[2K" + line)
	} else {
		r.writeLocked(line + "\n")
	}
}

func (r *semanticProgressRenderer) writeLocked(text string) {
	if r.writeErr != nil {
		return
	}
	written, err := io.WriteString(r.writer, text)
	if err == nil && written != len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		r.writeErr = fmt.Errorf("write semantic progress: %w", err)
	}
}

func (r *semanticProgressRenderer) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.writeErr
}

func (r *semanticProgressRenderer) Finish() error {
	r.mu.Lock()
	if r.finished {
		err := r.writeErr
		r.mu.Unlock()

		return err
	}
	r.finished = true
	if r.heartbeatStop != nil {
		close(r.heartbeatStop)
	}
	done := r.heartbeatDone
	if r.cold && r.tty {
		r.writeLocked("\n")
	}
	writeErr := r.writeErr
	r.mu.Unlock()
	if done != nil {
		<-done
	}

	return writeErr
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
