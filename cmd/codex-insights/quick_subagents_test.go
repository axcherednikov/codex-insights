package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func TestQuickSubagentsPreserveNonzeroUserStatsWithOtherSources(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "rollout-user.jsonl")
	writeJSONL(t, user,
		sessionMeta("user", "2026-10-01T00:00:00Z", "user", nil, "", "", ""),
		event("2026-10-01T00:00:01Z", "task_started", "u1", nil),
		makeSubagentContext("2026-10-01T00:00:02Z", "u1", "model-a", "high"),
		record("2026-10-01T00:00:03Z", "response_item", map[string]any{"type": "function_call", "name": "run"}),
		tokenCount("2026-10-01T00:00:04Z", 100),
		event("2026-10-01T00:00:05Z", "task_complete", "u1", map[string]any{"duration_ms": 5000}),
		event("2026-10-01T00:01:00Z", "task_started", "u2", nil),
		makeSubagentContext("2026-10-01T00:01:01Z", "u2", "model-b", "low"),
		tokenCount("2026-10-01T00:01:02Z", 130),
		event("2026-10-01T00:01:03Z", "turn_aborted", "u2", nil),
		event("2026-10-01T00:02:00Z", "task_started", "u3", nil),
		makeSubagentContext("2026-10-01T00:02:01Z", "u3", "model-c", "medium"),
	)
	ordinary := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, ordinary,
		sessionMeta("child", "2026-09-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 2, Role: "default"}, "", "", "root-session"),
		event("2026-10-01T00:03:00Z", "task_started", "followup", nil),
		makeSubagentContext("2026-10-01T00:03:01Z", "followup", "m", "high"),
		makeSubagentContext("2026-10-01T00:03:02Z", "followup", "m2", "high"),
		event("2026-10-01T00:03:03Z", "task_complete", "followup", nil),
		event("2026-10-01T00:04:00Z", "task_started", "second", nil),
		event("2026-10-01T00:04:30Z", "task_started", "aborted", nil),
		event("2026-10-01T00:04:31Z", "turn_aborted", "aborted", nil),
	)
	otherChild := filepath.Join(dir, "rollout-child-2.jsonl")
	writeJSONL(t, otherChild,
		sessionMeta("child-2", "2026-10-01T00:05:00Z", "subagent", &spawn{ID: "child-2", Parent: "root", Depth: 2, Role: "custom"}, "", "", "root-session"),
		event("2026-10-01T00:05:01Z", "task_started", "nested", nil),
	)
	guardian := filepath.Join(dir, "rollout-guardian.jsonl")
	writeJSONL(t, guardian, record("2026-10-01T00:00:00Z", "session_meta", map[string]any{"id": "guardian", "thread_source": "subagent", "source": map[string]any{"subagent": map[string]any{"other": "guardian"}}}))
	guardianReview := filepath.Join(dir, "rollout-guardian-review.jsonl")
	writeJSONL(t, guardianReview, record("2026-10-01T00:00:00Z", "session_meta", map[string]any{"id": "guardian-review", "thread_source": "subagent", "source": map[string]any{"subagent": map[string]any{"other": "guardian_review"}}}))
	review := filepath.Join(dir, "rollout-review.jsonl")
	writeJSONL(t, review, record("2026-10-01T00:00:00Z", "session_meta", map[string]any{"id": "review", "thread_source": "subagent", "source": map[string]any{"subagent": "review"}}))
	forked := filepath.Join(dir, "rollout-forked.jsonl")
	writeJSONL(t, forked,
		sessionMeta("forked-child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "forked-child", Parent: "root", Depth: 2, Role: "default"}, "parent-id", "", "root-session"),
		event("2026-10-01T00:06:00Z", "task_started", "inherited", nil),
		event("2026-10-01T00:06:01Z", "task_complete", "inherited", map[string]any{"trigger_turn": true}),
	)
	files := []string{user, ordinary, otherChild, guardian, guardianReview, review, forked}
	before := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	got := collectQuickStats(files, time.Time{}, before, "")
	wantUser := collectQuickStats([]string{user}, time.Time{}, before, "")
	project := func(s quickStats) quickStats { s.subagents = subagentStats{}; return s }
	if !reflect.DeepEqual(project(got), project(wantUser)) {
		t.Fatalf("user-only fields changed: mixed=%#v user=%#v", project(got), project(wantUser))
	}
	if got.sessionCount != 1 || got.turnCount != 3 || got.completedCount != 1 || got.totalTokens != 100 || got.totalDurationMS != 5000 || got.totalToolCalls != 1 {
		t.Fatalf("user fixture has weak/nonzero coverage: %#v", got)
	}
	if got.subagents.created != 3 || got.subagents.working != 4 || got.subagents.completed != 1 || got.subagents.aborted != 1 || got.subagents.incomplete != 2 || got.subagents.nested != 3 {
		t.Fatalf("subagent aggregate = %#v", got.subagents)
	}
	if got.subagents.roles["default"].Created != 2 || got.subagents.roles["default"].Working != 3 || got.subagents.roles["custom"].Created != 1 || got.subagents.roles["custom"].Working != 1 {
		t.Fatalf("role counts = %#v", got.subagents.roles)
	}
	if got.subagents.settings["m\thigh"] != 1 || got.subagents.settings["m2\thigh"] != 1 || got.subagents.settings["unknown\tunknown"] != 3 {
		t.Fatalf("settings = %#v", got.subagents.settings)
	}
}

func TestQuickSubagentsIndependentCreationAndWorkWindows(t *testing.T) {
	dir := t.TempDir()
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := since.Add(24 * time.Hour)
	older := filepath.Join(dir, "rollout-older.jsonl")
	writeJSONL(t, older, sessionMeta("older", since.Add(-time.Hour).Format(time.RFC3339), "subagent", &spawn{ID: "older", Parent: "root", Depth: 1, Role: "default"}, "", "", ""), event(since.Format(time.RFC3339), "task_started", "boundary", nil), event(before.Format(time.RFC3339), "task_complete", "boundary", nil), event(before.Add(time.Nanosecond).Format(time.RFC3339Nano), "task_started", "late", nil))
	newer := filepath.Join(dir, "rollout-newer.jsonl")
	writeJSONL(t, newer, sessionMeta("newer", before.Format(time.RFC3339), "subagent", &spawn{ID: "newer", Parent: "root", Depth: 1, Role: "default"}, "", "", ""))
	stats := collectQuickSubagents([]string{older, newer}, since, before, "")
	if stats.created != 1 || stats.working != 1 || stats.completed != 1 || stats.incomplete != 0 {
		t.Fatalf("independent cohorts = %#v", stats)
	}
}

func TestQuickSubagentsMissingFieldsAndUnknownSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-missing.jsonl")
	writeJSONL(t, path, sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child"}, "", "", ""), event("2026-10-01T00:00:01Z", "task_started", "turn", nil), event("2026-10-01T00:00:02Z", "task_complete", "turn", nil))
	missingID := filepath.Join(dir, "rollout-no-id.jsonl")
	writeJSONL(t, missingID, sessionMeta("", "2026-10-01T00:00:00Z", "subagent", &spawn{}, "", "", ""))
	stats := collectQuickSubagents([]string{path, missingID}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.created != 1 || stats.working != 1 || stats.missingIDs != 1 || stats.missingParents != 1 || stats.missingDepth != 1 || stats.roles["unknown"].Created != 1 || stats.settings["unknown\tunknown"] != 1 {
		t.Fatalf("missing fields = %#v", stats)
	}
}

func TestQuickSubagentsForkOwnershipAndCopiedHistory(t *testing.T) {
	for _, tc := range []struct {
		name, owner                                 string
		duplicate                                   bool
		wantWorking, wantUnattributed, wantDisputes int
	}{
		{name: "foreign", owner: "parent", wantUnattributed: 1},
		{name: "owned", owner: "child", wantWorking: 1},
		{name: "conflict", owner: "child", duplicate: true, wantUnattributed: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			child := filepath.Join(dir, "rollout-child.jsonl")
			records := []any{sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "fork", "", ""), event("2026-10-01T00:00:01Z", "task_started", "turn", nil), record("2026-10-01T00:00:02Z", "token_usage_record", map[string]any{"thread_id": tc.owner, "turn_id": "turn"})}
			if tc.duplicate {
				records = append(records, record("2026-10-01T00:00:03Z", "token_usage_record", map[string]any{"thread_id": "parent", "turn_id": "turn"}))
			}
			writeJSONL(t, child, records...)
			stats := collectQuickSubagents([]string{child}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
			if stats.working != tc.wantWorking || stats.unattributed != tc.wantUnattributed || stats.disputes != tc.wantDisputes {
				t.Fatalf("fork stats = %#v", stats)
			}
		})
	}
}

func TestQuickSubagentsReconcilePrefixFullAndMetadataDisputes(t *testing.T) {
	t.Run("prefix full", func(t *testing.T) {
		dir := t.TempDir()
		prefix := filepath.Join(dir, "rollout-prefix.jsonl")
		full := filepath.Join(dir, "rollout-full.jsonl")
		meta := sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 2, Role: "default"}, "", "", "")
		start := event("2026-10-01T00:00:01Z", "task_started", "turn", nil)
		terminal := event("2026-10-01T00:00:02Z", "task_complete", "turn", nil)
		writeJSONL(t, prefix, meta, start)
		writeJSONL(t, full, meta, start, terminal)
		stats := collectQuickSubagents([]string{prefix, full}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
		if stats.created != 1 || stats.working != 1 || stats.completed != 1 || stats.terminalMismatches != 0 {
			t.Fatalf("prefix/full = %#v", stats)
		}
	})
	t.Run("role conflict preserves confirmed creation", func(t *testing.T) {
		dir := t.TempDir()
		a := filepath.Join(dir, "rollout-a.jsonl")
		b := filepath.Join(dir, "rollout-b.jsonl")
		metaA := sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 2, Role: "one"}, "", "", "")
		metaB := sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 2, Role: "two"}, "", "", "")
		start := event("2026-10-01T00:00:01Z", "task_started", "turn", nil)
		writeJSONL(t, a, metaA, start)
		writeJSONL(t, b, metaB, start)
		stats := collectQuickSubagents([]string{a, b}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
		if stats.created != 1 || stats.roles["unknown"].Created != 1 || stats.working != 0 || stats.disputes == 0 {
			t.Fatalf("role conflict = %#v", stats)
		}
	})
	t.Run("fork conflict excludes work", func(t *testing.T) {
		dir := t.TempDir()
		a := filepath.Join(dir, "rollout-a.jsonl")
		b := filepath.Join(dir, "rollout-b.jsonl")
		writeJSONL(t, a, sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "", "", ""), event("2026-10-01T00:00:01Z", "task_started", "turn", nil))
		writeJSONL(t, b, sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "fork", "", ""), event("2026-10-01T00:00:01Z", "task_started", "turn", nil), record("2026-10-01T00:00:02Z", "token_usage_record", map[string]any{"thread_id": "child", "turn_id": "turn"}))
		stats := collectQuickSubagents([]string{a, b}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
		if stats.created != 1 || stats.working != 0 || stats.disputes == 0 {
			t.Fatalf("fork conflict = %#v", stats)
		}
	})
}

func TestQuickSubagentsDeduplicatesOrphanTerminals(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "rollout-a.jsonl")
	b := filepath.Join(dir, "rollout-b.jsonl")
	meta := sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "", "", "")
	orphan := event("2026-10-01T00:00:01Z", "task_complete", "missing-start", nil)
	writeJSONL(t, a, meta, orphan, orphan)
	writeJSONL(t, b, meta, orphan)
	stats := collectQuickSubagents([]string{a, b}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.terminalMismatches != 1 {
		t.Fatalf("orphan terminal count = %d, want 1: %#v", stats.terminalMismatches, stats)
	}
}

func TestQuickSubagentsContradictoryTerminalEventsExcludeTurn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, path, sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "", "", ""), event("2026-10-01T00:00:01Z", "task_started", "turn", nil), event("2026-10-01T00:00:02Z", "task_complete", "turn", nil), event("2026-10-01T00:00:02Z", "turn_aborted", "turn", nil))
	stats := collectQuickSubagents([]string{path}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.working != 0 || stats.disputes != 1 || stats.terminalMismatches != 0 {
		t.Fatalf("contradictory terminal status = %#v", stats)
	}
}

func TestQuickSubagentsSurfacesMalformedRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-child.jsonl")
	writeRawJSONL(t, path, sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "", "", ""), "not json", event("2026-10-01T00:00:01Z", "task_started", "turn", nil))
	stats := collectQuickSubagents([]string{path}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.malformed != 1 || stats.working != 1 {
		t.Fatalf("malformed record handling = %#v", stats)
	}
}

func TestQuickSubagentsSkipsFailedRolloutButUsesValidDuplicate(t *testing.T) {
	dir := t.TempDir()
	failed := filepath.Join(dir, "rollout-failed.jsonl")
	valid := filepath.Join(dir, "rollout-valid.jsonl")
	meta := sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "default"}, "", "", "")
	oversized := strings.Repeat("x", 16*1024*1024+1)
	writeRawJSONL(t, failed, meta, oversized)
	stats := collectQuickSubagents([]string{failed}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.created != 0 || stats.readErrors != 1 {
		t.Fatalf("failed file counted: %#v", stats)
	}
	writeJSONL(t, valid, meta, event("2026-10-01T00:00:01Z", "task_started", "turn", nil))
	stats = collectQuickSubagents([]string{failed, valid}, time.Time{}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "")
	if stats.created != 1 || stats.working != 1 || stats.readErrors != 1 {
		t.Fatalf("valid duplicate not retained: %#v", stats)
	}
}

func TestQuickSubagentsLocalizesZeroAndPopulatedSections(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{{"en", "Subagents"}, {"ru", "Субагенты"}} {
		tr, _ := i18n.New(tc.lang)
		for _, stats := range []subagentStats{{roles: map[string]subagentRoleCounts{}, settings: map[string]int{}}, {created: 1, working: 1, roles: map[string]subagentRoleCounts{"default": {Created: 1, Working: 1}}, settings: map[string]int{"m\thigh": 1}}} {
			output := captureOutput(t, func() { printQuickSubagents(tr, stats) })
			if !strings.Contains(output, tc.want) {
				t.Fatalf("%s report = %q", tc.lang, output)
			}
		}
	}
}

type spawn struct {
	ID, Parent, Role string
	Depth            int
}

func sessionMeta(id, timestamp, threadSource string, spawnData *spawn, forkedFrom, originator, sessionID string) map[string]any {
	payload := map[string]any{"id": id, "thread_source": threadSource, "originator": originator, "session_id": sessionID}
	if forkedFrom != "" {
		payload["forked_from_id"] = forkedFrom
		payload["subagent_history_start_ordinal"] = 999
	}
	if spawnData != nil {
		tree := map[string]any{"id": spawnData.ID, "parent_thread_id": spawnData.Parent, "agent_role": spawnData.Role}
		if spawnData.Depth != 0 {
			tree["depth"] = spawnData.Depth
		}
		payload["source"] = map[string]any{"subagent": map[string]any{"thread_spawn": tree}}
	}
	return record(timestamp, "session_meta", payload)
}
func event(timestamp, typ, turnID string, extra map[string]any) map[string]any {
	payload := map[string]any{"type": typ}
	if turnID != "" {
		payload["turn_id"] = turnID
	}
	for k, v := range extra {
		payload[k] = v
	}
	return record(timestamp, "event_msg", payload)
}
func makeSubagentContext(timestamp, turnID, model, effort string) map[string]any {
	return record(timestamp, "turn_context", map[string]any{"turn_id": turnID, "model": model, "effort": effort})
}
func tokenCount(timestamp string, total int) map[string]any {
	return record(timestamp, "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": map[string]any{"total_tokens": total}}})
}
func record(timestamp, typ string, payload any) map[string]any {
	return map[string]any{"timestamp": timestamp, "type": typ, "payload": payload}
}
func writeJSONL(t *testing.T, path string, records ...any) {
	t.Helper()
	var b strings.Builder
	for _, item := range records {
		data, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
func writeRawJSONL(t *testing.T, path string, records ...any) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range records {
		if raw, ok := item.(string); ok {
			_, err = f.WriteString(raw + "\n")
		} else {
			data, e := json.Marshal(item)
			if e != nil {
				t.Fatal(e)
			}
			_, err = f.Write(data)
			if err == nil {
				_, err = f.WriteString("\n")
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return string(data)
}

func TestQuickSubagentsReportRecordedTokensAndWorkingTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-usage.jsonl")
	writeJSONL(t, path,
		sessionMeta("child", "2026-10-01T00:00:00Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1, Role: "custom"}, "", "", ""),
		event("2026-10-01T00:00:01Z", "task_started", "turn", nil),
		makeSubagentContext("2026-10-01T00:00:02Z", "turn", "actual-model", "high"),
		record("2026-10-01T00:00:03Z", "token_usage_record", map[string]any{"thread_id": "child", "turn_id": "turn", "response_id": "response", "usage": map[string]any{"input_tokens": 100, "cached_input_tokens": 80, "output_tokens": 10, "reasoning_output_tokens": 4, "total_tokens": 110}}),
		event("2026-10-01T00:00:04Z", "task_complete", "turn", map[string]any{"duration_ms": 3000}),
	)
	stats := collectQuickSubagents([]string{path}, time.Time{}, time.Time{}, "")
	tr, _ := i18n.New("en")
	output := captureOutput(t, func() { printQuickSubagents(tr, stats) })
	if !strings.Contains(output, "token_usage_record") || !strings.Contains(output, "Recorded working time") {
		t.Fatalf("missing recorded resource statistics: %s", output)
	}
}
