package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/axcherednikov/codex-insights/internal/i18n"
)

const resourceDate = "2026-10-01T00:00:"

func resourceUsage(input, cached, output, reasoning int64) map[string]any {
	return map[string]any{"input_tokens": input, "cached_input_tokens": cached, "output_tokens": output, "reasoning_output_tokens": reasoning, "total_tokens": input + output}
}

func resourceRequest(at, child, turn, response string, usage any) map[string]any {
	return record(resourceDate+at+"Z", "token_usage_record", map[string]any{"thread_id": child, "turn_id": turn, "response_id": response, "usage": usage,
		"turn_token_usage": resourceUsage(9000, 0, 0, 0), "thread_token_usage": resourceUsage(99000, 0, 0, 0)})
}

func resourceCounter(at string, usage any) map[string]any {
	return record(resourceDate+at+"Z", "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": usage, "last_token_usage": resourceUsage(999, 0, 0, 0)}})
}

func resourceStart(child, turn, role, fork string) []any {
	return []any{sessionMeta(child, resourceDate+"00Z", "subagent", &spawn{ID: child, Parent: "root", Depth: 2, Role: role}, fork, "", "shared-root"),
		event(resourceDate+"01Z", "task_started", turn, nil), makeSubagentContext(resourceDate+"02Z", turn, "actual-model", "high")}
}

func resourceCollect(t *testing.T, records ...any) subagentStats {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout-resource.jsonl")
	writeJSONL(t, path, records...)
	return collectQuickSubagents([]string{path}, time.Time{}, time.Time{}, "")
}

func TestQuickSubagentResourcesDeduplicateAndKeepSourcesSeparate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		counter  map[string]any
		mismatch int
	}{
		{"matching", resourceUsage(100, 80, 10, 4), 0},
		{"total differs", resourceUsage(120, 80, 10, 4), 1},
		{"same total subsets differ", resourceUsage(100, 70, 10, 3), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			request := resourceRequest("03", "child", "turn", "response", resourceUsage(100, 80, 10, 4))
			terminal := event(resourceDate+"05Z", "task_complete", "turn", map[string]any{"duration_ms": 3000})
			records := append(resourceStart("child", "turn", "custom", ""), request, request, resourceCounter("04", tc.counter), terminal, terminal)
			path := filepath.Join(dir, "rollout-one.jsonl")
			copyPath := filepath.Join(dir, "rollout-copy.jsonl")
			writeJSONL(t, path, records...)
			writeJSONL(t, copyPath, records...)
			stats := collectQuickSubagents([]string{path, copyPath, path}, time.Time{}, time.Time{}, "")
			s := stats.resources
			if stats.working != 1 || s.recorded.total != 110 || s.estimated.known || s.recordedTurns != 1 || s.mismatchedCounters != tc.mismatch || s.invalidRecords != 0 {
				t.Fatalf("sources or duplicate totals: %#v", s)
			}
			if s.recorded.parts != [4]int64{100, 80, 10, 4} || s.durationMS != 3000 || s.durationTurns != 1 {
				t.Fatalf("inclusive components/duration: %#v", s)
			}
			if s.byRole["custom"].recorded.total != 110 || s.byModel["actual-model"].recorded.total != 110 {
				t.Fatal("recorded attribution missing")
			}
		})
	}
}

func TestQuickSubagentResourcesLegacyCumulativeCounters(t *testing.T) {
	records := []any{sessionMeta("child", resourceDate+"00Z", "subagent", &spawn{ID: "child", Role: "old-role", Depth: 1}, "", "", ""), resourceCounter("01", resourceUsage(100, 80, 10, 4)),
		event(resourceDate+"02Z", "task_started", "first", nil), makeSubagentContext(resourceDate+"03Z", "first", "old-model", "medium"),
		resourceCounter("04", resourceUsage(120, 90, 15, 5)), resourceCounter("05", resourceUsage(140, 100, 20, 6)), resourceCounter("05", resourceUsage(140, 100, 20, 6)),
		event(resourceDate+"06Z", "task_complete", "first", map[string]any{"duration_ms": 4000}),
		event(resourceDate+"07Z", "task_started", "followup", nil), makeSubagentContext(resourceDate+"08Z", "followup", "old-model", "medium"),
		resourceCounter("09", resourceUsage(180, 120, 30, 8)), event(resourceDate+"10Z", "turn_aborted", "followup", nil)}
	s := resourceCollect(t, records...).resources
	if s.recorded.known || s.estimated.total != 100 || s.estimatedTurns != 2 || s.missingTurns != 0 || s.invalidCounters != 0 {
		t.Fatalf("legacy delta totals: %#v", s)
	}
	if s.estimated.parts != [4]int64{80, 40, 20, 4} || s.byModel["old-model"].estimated.total != 100 || s.durationMS != 4000 || s.missingDuration != 1 {
		t.Fatalf("legacy components/time: %#v", s)
	}
}

func TestQuickSubagentResourcesCounterResetAndMissingBaseline(t *testing.T) {
	for _, tc := range []struct {
		name               string
		middle             []any
		invalid, estimated int
		tokens             int64
	}{
		{"reset within turn", []any{resourceCounter("03", resourceUsage(100, 0, 10, 0)), resourceCounter("04", resourceUsage(5, 0, 0, 0))}, 1, 0, 0},
		{"missing baseline after compaction", []any{record(resourceDate+"00Z", "compacted", nil), event(resourceDate+"01Z", "task_started", "new", nil), resourceCounter("03", resourceUsage(100, 0, 10, 0))}, 1, 0, 0},
		{"total only", []any{resourceCounter("03", map[string]any{"total_tokens": 10}), resourceCounter("04", map[string]any{"total_tokens": 20})}, 0, 1, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := resourceStart("child", "turn", "role", "")
			if tc.name == "missing baseline after compaction" {
				records = records[:1]
			}
			s := resourceCollect(t, append(records, tc.middle...)...).resources
			if s.invalidCounters != tc.invalid || s.estimatedTurns != tc.estimated || s.estimated.total != tc.tokens {
				t.Fatalf("counter state: %#v", s)
			}
		})
	}
	// A reset observed between turns is a new baseline, not new expenditure.
	records := append(resourceStart("child", "first", "role", ""), resourceCounter("03", map[string]any{"total_tokens": 100}), event(resourceDate+"04Z", "task_complete", "first", nil), resourceCounter("05", map[string]any{"total_tokens": 5}), event(resourceDate+"06Z", "task_started", "second", nil), resourceCounter("07", map[string]any{"total_tokens": 15}))
	s := resourceCollect(t, records...).resources
	if s.estimated.total != 110 || s.estimatedTurns != 2 || s.invalidCounters != 0 {
		t.Fatalf("between-turn baseline reset: %#v", s)
	}
}

func TestQuickSubagentResourcesExcludeInheritedNestedHistory(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "rollout-parent.jsonl")
	child := filepath.Join(dir, "rollout-child.jsonl")
	inherited := resourceRequest("03", "parent", "parent-turn", "parent-response", resourceUsage(100, 50, 10, 3))
	writeJSONL(t, parent, append(resourceStart("parent", "parent-turn", "parent-role", ""), inherited, event(resourceDate+"04Z", "task_complete", "parent-turn", map[string]any{"duration_ms": 2000}))...)
	records := []any{sessionMeta("child", resourceDate+"00Z", "subagent", &spawn{ID: "child", Parent: "parent", Depth: 2, Role: "child-role"}, "parent", "", "shared-root"),
		event(resourceDate+"01Z", "task_started", "parent-turn", nil), inherited, event(resourceDate+"04Z", "task_complete", "parent-turn", map[string]any{"duration_ms": 2000}),
		event(resourceDate+"05Z", "task_started", "owned", nil), makeSubagentContext(resourceDate+"06Z", "owned", "child-model", "low"), resourceRequest("07", "child", "owned", "child-response", resourceUsage(10, 0, 1, 0)), resourceCounter("08", resourceUsage(10000, 9000, 100, 50)), event(resourceDate+"09Z", "task_complete", "owned", map[string]any{"duration_ms": 1000})}
	writeJSONL(t, child, records...)
	s := collectQuickSubagents([]string{parent, child}, time.Time{}, time.Time{}, "")
	if s.working != 2 || s.resources.recorded.total != 121 || s.resources.durationMS != 3000 || s.resources.estimated.known || s.unattributed != 1 {
		t.Fatalf("nested inherited resources: %#v", s)
	}
	if s.resources.byRole["parent-role"].recorded.total != 110 || s.resources.byRole["child-role"].recorded.total != 11 {
		t.Fatal("roles duplicated inherited expenditure")
	}
	old := resourceCollect(t, append(resourceStart("old-fork", "unconfirmed", "role", "parent"), resourceCounter("03", resourceUsage(10000, 0, 0, 0)), event(resourceDate+"04Z", "task_complete", "unconfirmed", map[string]any{"duration_ms": 99999}))...)
	if old.working != 0 || old.unattributed != 1 || old.resources.recorded.known || old.resources.estimated.known || old.resources.durationTurns != 0 {
		t.Fatalf("old fork guessed ownership: %#v", old)
	}
}

func TestQuickSubagentResourcesMissingInvalidAndConflictingRecords(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records []any
		tokens  int64
		known   bool
		invalid int
	}{
		{"absent", nil, 0, false, 0},
		{"missing response identity", []any{resourceRequest("03", "child", "turn", "", resourceUsage(100, 0, 10, 0))}, 0, false, 1},
		{"foreign thread", []any{resourceRequest("03", "parent", "turn", "r", resourceUsage(100, 0, 10, 0))}, 0, false, 0},
		{"invalid subset", []any{resourceRequest("03", "child", "turn", "r", resourceUsage(100, 101, 10, 0))}, 0, false, 1},
		{"invalid field type", []any{resourceRequest("03", "child", "turn", "r", map[string]any{"total_tokens": "110"})}, 0, false, 1},
		{"conflicting response", []any{resourceRequest("03", "child", "turn", "r", resourceUsage(100, 0, 10, 0)), resourceRequest("03", "child", "turn", "r", resourceUsage(200, 0, 10, 0))}, 0, false, 1},
		{"total-only breakdown missing", []any{resourceRequest("03", "child", "turn", "r", map[string]any{"total_tokens": 110})}, 110, true, 0},
		{"compatible partial duplicate", []any{resourceRequest("03", "child", "turn", "r", map[string]any{"total_tokens": 110}), resourceRequest("03", "child", "turn", "r", resourceUsage(100, 80, 10, 4))}, 110, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := resourceCollect(t, append(resourceStart("child", "turn", "role", ""), tc.records...)...).resources
			if s.recorded.total != tc.tokens || s.recorded.known != tc.known || s.invalidRecords != tc.invalid || s.estimated.known {
				t.Fatalf("request evidence: %#v", s)
			}
			if !tc.known && s.missingTurns != 1 {
				t.Fatalf("missing evidence not disclosed: %#v", s)
			}
			if tc.name == "total-only breakdown missing" && s.recorded.missing != [4]bool{true, true, true, true} {
				t.Fatal("absent components treated as zero")
			}
		})
	}
	// Even a valid fallback counter cannot silently replace conflicting records.
	records := append(resourceStart("child", "turn", "role", ""), resourceRequest("03", "child", "turn", "r", nil), resourceCounter("04", resourceUsage(100, 0, 10, 0)))
	s := resourceCollect(t, records...).resources
	if s.estimated.known || s.missingTurns != 1 {
		t.Fatalf("unusable request silently replaced: %#v", s)
	}
}

func TestQuickSubagentResourcesAssignModelsAtRequestTime(t *testing.T) {
	records := append(resourceStart("child", "turn", "role", ""), resourceRequest("03", "child", "turn", "r1", resourceUsage(100, 0, 10, 0)), makeSubagentContext(resourceDate+"04Z", "turn", "other-model", "low"), resourceRequest("05", "child", "turn", "r2", resourceUsage(10, 0, 1, 0)))
	s := resourceCollect(t, records...).resources
	if s.recorded.total != 121 || s.byModel["actual-model"].recorded.total != 110 || s.byModel["other-model"].recorded.total != 11 {
		t.Fatalf("actual model attribution: %#v", s)
	}
	// A cumulative turn total cannot be divided between different models.
	records = append(resourceStart("child", "turn", "role", ""), makeSubagentContext(resourceDate+"04Z", "turn", "other-model", "low"), resourceCounter("05", resourceUsage(100, 0, 10, 0)))
	s = resourceCollect(t, records...).resources
	if s.byModel["unknown"].estimated.total != 110 || len(s.byModel) != 1 {
		t.Fatalf("guessed fallback model: %#v", s)
	}
}

func TestQuickSubagentResourcesRejectResponseIdentityReusedByChildren(t *testing.T) {
	dir := t.TempDir()
	files := []string{}
	for _, child := range []string{"one", "two"} {
		path := filepath.Join(dir, "rollout-"+child+".jsonl")
		writeJSONL(t, path, append(resourceStart(child, "turn", "role", ""), resourceRequest("03", child, "turn", "same-response", resourceUsage(100, 0, 10, 0)))...)
		files = append(files, path)
	}
	s := collectQuickSubagents(files, time.Time{}, time.Time{}, "").resources
	if s.recorded.known || s.invalidRecords != 2 || s.missingTurns != 2 {
		t.Fatalf("shared response double counted: %#v", s)
	}
}

func TestQuickSubagentResourcesRecordedDurationAndCutoff(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		durations               []any
		status                  string
		known, missing, invalid int
		ms                      int64
	}{
		{"missing", []any{nil}, "task_complete", 0, 1, 0, 0},
		{"zero", []any{int64(0)}, "task_complete", 1, 0, 0, 0},
		{"negative", []any{int64(-1)}, "task_complete", 0, 1, 1, 0},
		{"wrong type", []any{"1000"}, "task_complete", 0, 1, 1, 0},
		{"duplicate", []any{int64(1000), int64(1000)}, "task_complete", 1, 0, 0, 1000},
		{"partial duplicate", []any{nil, int64(1000)}, "task_complete", 1, 0, 0, 1000},
		{"conflicting", []any{int64(1000), int64(2000)}, "task_complete", 0, 1, 1, 0},
		{"aborted", []any{int64(1000)}, "turn_aborted", 0, 1, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := resourceStart("child", "turn", "role", "")
			for _, ms := range tc.durations {
				records = append(records, event(resourceDate+"05Z", tc.status, "turn", map[string]any{"duration_ms": ms}))
			}
			stats := resourceCollect(t, records...)
			s := stats.resources
			if s.durationMS != tc.ms || s.durationTurns != tc.known || s.missingDuration != tc.missing || s.invalidDuration != tc.invalid {
				t.Fatalf("duration evidence: %#v", s)
			}
			if tc.status == "task_complete" && stats.completed != 1 {
				t.Fatal("invalid duration changed lifecycle status")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "rollout-cutoff.jsonl")
	writeJSONL(t, path, append(resourceStart("child", "turn", "role", ""), resourceRequest("03", "child", "turn", "before", resourceUsage(10, 0, 1, 0)), resourceRequest("06", "child", "turn", "after", resourceUsage(100, 0, 10, 0)), event(resourceDate+"07Z", "task_complete", "turn", map[string]any{"duration_ms": 5000}))...)
	before, _ := time.Parse(time.RFC3339, resourceDate+"05Z")
	stats := collectQuickSubagents([]string{path}, time.Time{}, before, "")
	if stats.incomplete != 1 || stats.resources.recorded.total != 11 || stats.resources.missingDuration != 1 {
		t.Fatalf("future records leaked: %#v", stats)
	}
	since, _ := time.Parse(time.RFC3339, resourceDate+"02Z")
	if s := collectQuickSubagents([]string{path}, since, time.Time{}, "").resources; s.recorded.known || s.durationTurns != 0 {
		t.Fatal("resources ignored start cohort window")
	}
}

func TestQuickSubagentResourcesParallelTimeDoesNotChangeUserStats(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "rollout-user.jsonl")
	writeJSONL(t, user, sessionMeta("user", resourceDate+"00Z", "user", nil, "", "", ""), event(resourceDate+"01Z", "task_started", "user-turn", nil), tokenCount(resourceDate+"03Z", 100), event(resourceDate+"05Z", "task_complete", "user-turn", map[string]any{"duration_ms": 4000}))
	files := []string{user}
	for _, child := range []string{"one", "two"} {
		path := filepath.Join(dir, "rollout-"+child+".jsonl")
		writeJSONL(t, path, append(resourceStart(child, "turn", "role", ""), resourceRequest("03", child, "turn", "response-"+child, resourceUsage(10, 0, 1, 0)), event(resourceDate+"05Z", "task_complete", "turn", map[string]any{"duration_ms": 4000}))...)
		files = append(files, path)
	}
	before := parseMustTime(resourceDate + "10Z")
	baseline := collectQuickStats([]string{user}, time.Time{}, before, "")
	got := collectQuickStats(files, time.Time{}, before, "")
	project := func(s quickStats) quickStats { s.subagents = subagentStats{}; return s }
	if !reflect.DeepEqual(project(got), project(baseline)) || got.totalDurationMS != 4000 || got.totalTokens != 100 {
		t.Fatal("agent resources changed user statistics")
	}
	if got.subagents.resources.durationMS != 8000 || got.subagents.resources.recorded.total != 22 {
		t.Fatal("parallel agent work not separately summed")
	}
}

func TestQuickSubagentResourcesLocalizedMissingData(t *testing.T) {
	for _, tc := range []struct{ lang, duration, unknown string }{{"en", "Recorded working time", "n/a"}, {"ru", "Записанное рабочее время", "н/д"}} {
		tr, _ := i18n.New(tc.lang)
		s := resourceCollect(t, resourceStart("child", "turn", "role", "")...).resources
		output := captureOutput(t, func() { printSubagentResources(tr, s) })
		if !strings.Contains(output, tc.duration+": "+tc.unknown) || !strings.Contains(output, "token_usage_record") || !strings.Contains(output, "token_count") {
			t.Fatalf("source/absence labels: %s", output)
		}
	}
}

func TestQuickSubagentResourcesPrimarySurvivesInvalidLegacyData(t *testing.T) {
	for _, count := range []any{
		resourceCounter("04", map[string]any{"total_tokens": "invalid"}),
		record(resourceDate+"04Z", "event_msg", map[string]any{"type": "token_count", "turn_id": "other", "info": map[string]any{"total_token_usage": resourceUsage(100, 0, 10, 0)}}),
	} {
		records := append(resourceStart("child", "turn", "role", ""), resourceRequest("03", "child", "turn", "r", resourceUsage(100, 0, 10, 0)), count)
		s := resourceCollect(t, records...).resources
		if s.recorded.total != 110 || s.estimated.known || s.invalidCounters != 1 {
			t.Fatalf("invalid counter replaced trusted request: %#v", s)
		}
	}
}

func TestQuickSubagentResourcesMissingModelIsUnattributed(t *testing.T) {
	records := resourceStart("child", "turn", "", "parent")[:2]
	records = append(records, resourceRequest("03", "child", "turn", "r", resourceUsage(100, 0, 10, 0)))
	stats := resourceCollect(t, records...)
	if stats.working != 1 || stats.resources.byModel["unknown"].recorded.total != 110 || stats.resources.byRole["unknown"].recorded.total != 110 {
		t.Fatalf("missing context inferred: %#v", stats.resources)
	}
}

func TestQuickSubagentResourcesRejectConflictingCounterSnapshots(t *testing.T) {
	records := append(resourceStart("child", "turn", "role", ""), resourceCounter("03", resourceUsage(100, 0, 10, 0)), resourceCounter("03", resourceUsage(120, 0, 10, 0)))
	s := resourceCollect(t, records...).resources
	if s.estimated.known || s.invalidCounters != 1 || s.missingTurns != 1 {
		t.Fatalf("conflicting snapshots arbitrarily chosen: %#v", s)
	}
}

func TestQuickSubagentResourcesOverflowIsUnavailable(t *testing.T) {
	records := append(resourceStart("child", "turn", "role", ""),
		resourceRequest("03", "child", "turn", "r1", map[string]any{"total_tokens": int64(1<<63 - 1)}),
		resourceRequest("04", "child", "turn", "r2", map[string]any{"total_tokens": int64(1)}))
	s := resourceCollect(t, records...).resources
	tr, _ := i18n.New("en")
	output := captureOutput(t, func() { printSubagentResources(tr, s) })
	if !s.recorded.overflow || !strings.Contains(output, "Tokens (token_usage_record): n/a") || !strings.Contains(output, "exceeds the supported integer range") {
		t.Fatalf("aggregate overflow concealed: %s", output)
	}
}

func TestQuickSubagentResourcesOverlappingLegacyTurnsAreUnattributed(t *testing.T) {
	records := append(resourceStart("child", "first", "role", ""), resourceCounter("03", resourceUsage(100, 0, 10, 0)), event(resourceDate+"04Z", "task_started", "second", nil), resourceCounter("05", resourceUsage(120, 0, 10, 0)))
	s := resourceCollect(t, records...).resources
	if s.estimated.known || s.invalidCounters != 2 || s.missingTurns != 2 {
		t.Fatalf("ambiguous cumulative ownership guessed: %#v", s)
	}
}

func TestQuickSubagentResourcesDurationVariantsDoNotDuplicateOrphanEvents(t *testing.T) {
	records := []any{sessionMeta("child", resourceDate+"00Z", "subagent", &spawn{ID: "child", Role: "role", Depth: 1}, "", "", ""), event(resourceDate+"04Z", "task_complete", "orphan", nil), event(resourceDate+"04Z", "task_complete", "orphan", map[string]any{"duration_ms": 1000})}
	s := resourceCollect(t, records...)
	if s.terminalMismatches != 1 || s.resources.durationTurns != 0 {
		t.Fatalf("duration variants counted orphan twice: %#v", s)
	}
}

func TestQuickSubagentResourcesEquivalentCounterTimestamps(t *testing.T) {
	records := append(resourceStart("child", "turn", "role", ""), resourceCounter("03", resourceUsage(100, 0, 10, 0)),
		record(resourceDate+"03+00:00", "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": resourceUsage(120, 0, 10, 0)}}))
	s := resourceCollect(t, records...).resources
	if s.estimated.known || s.invalidCounters != 1 {
		t.Fatalf("same instant treated as separate counters: %#v", s)
	}
}
