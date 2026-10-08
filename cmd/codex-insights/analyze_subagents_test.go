package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

func rootedContext(at, turn, root, model string) map[string]any {
	return record(resourceDate+at+"Z", "turn_context", map[string]any{"turn_id": turn, "root_turn_id": root, "model": model, "effort": "high"})
}

func rootedRequest(at, child, turn, root, response string, tokens int64) map[string]any {
	r := resourceRequest(at, child, turn, response, resourceUsage(tokens, tokens/2, 10, 4))
	r["payload"].(map[string]any)["root_turn_id"] = root

	return r
}

func effectivenessUser(t *testing.T, dir string) (string, collectedSessions) {
	t.Helper()
	path := filepath.Join(dir, "rollout-user.jsonl")
	writeJSONL(t, path, sessionMeta("root", resourceDate+"00Z", "user", nil, "", "", ""),
		event(resourceDate+"01Z", "task_started", "u1", nil), event(resourceDate+"02Z", "user_message", "u1", map[string]any{"message": "synthetic first task"}),
		rootedContext("02", "u1", "u1", "parent-model"), rootedRequest("11", "root", "u1", "u1", "p1", 20),
		resourceCounter("11", resourceUsage(200, 100, 10, 4)),
		event(resourceDate+"12Z", "task_complete", "u1", map[string]any{"duration_ms": 10000, "last_agent_message": "synthetic answer"}),
		event(resourceDate+"20Z", "task_started", "u2", nil), event(resourceDate+"21Z", "user_message", "u2", map[string]any{"message": "synthetic followup"}),
		rootedContext("21", "u2", "u2", "parent-model"), resourceCounter("38", resourceUsage(230, 100, 20, 4)),
		event(resourceDate+"40Z", "task_complete", "u2", map[string]any{"duration_ms": 20000, "last_agent_message": "synthetic followup answer"}))

	return path, collectSessionFiles([]string{path}, sessionWindow{Before: parseMustTime(resourceDate + "59Z").Add(time.Hour)})
}

func TestAnalyzeSubagentsNestedReuseDuplicatesAndUserIsolation(t *testing.T) {
	dir := t.TempDir()
	user, collected := effectivenessUser(t, dir)
	child := filepath.Join(dir, "rollout-child.jsonl")
	first := rootedRequest("05", "child", "c1", "u1", "c-response", 100)
	call := record(resourceDate+"04Z", "response_item", map[string]any{"type": "function_call", "call_id": "call-c"})
	terminal := event(resourceDate+"10Z", "task_complete", "c1", map[string]any{"duration_ms": 8000})
	childRecords := []any{sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: "root", Role: "reviewer", Depth: 1}, "", "", "shared"),
		event(resourceDate+"03Z", "task_started", "c1", nil), rootedContext("04", "c1", "u1", "actual"), first, first, call, call,
		resourceCounter("09", resourceUsage(999, 0, 0, 0)), terminal, terminal,
		event(resourceDate+"22Z", "task_started", "c2", nil), rootedContext("23", "c2", "u2", "other-actual"), rootedRequest("24", "child", "c2", "u2", "c-response-2", 50),
		event(resourceDate+"30Z", "task_complete", "c2", map[string]any{"duration_ms": 7000})}
	writeJSONL(t, child, childRecords...)
	copyPath := filepath.Join(dir, "rollout-child-copy.jsonl")
	writeJSONL(t, copyPath, childRecords...)
	nested := filepath.Join(dir, "rollout-nested.jsonl")
	writeJSONL(t, nested, sessionMeta("nested", resourceDate+"04Z", "subagent", &spawn{ID: "nested", Parent: "child", Role: "researcher", Depth: 2}, "child", "", "shared"),
		// Fork-inherited history is inside the time window but belongs to child.
		event(resourceDate+"03Z", "task_started", "c1", nil), rootedContext("04", "c1", "u1", "copied-model"), first, terminal,
		event(resourceDate+"05Z", "task_started", "n1", nil), rootedContext("06", "n1", "u1", "nested-model"), rootedRequest("07", "nested", "n1", "u1", "n-response", 200),
		record(resourceDate+"08Z", "response_item", map[string]any{"type": "custom_tool_call", "call_id": "n-call"}),
		event(resourceDate+"09Z", "task_complete", "n1", map[string]any{"duration_ms": 4000}))
	guardian := filepath.Join(dir, "rollout-guardian.jsonl")
	writeJSONL(t, guardian, record(resourceDate+"00Z", "session_meta", map[string]any{"id": "guardian", "thread_source": "subagent", "source": map[string]any{"subagent": map[string]any{"other": "guardian"}}}), event(resourceDate+"05Z", "task_started", "g", nil))
	files := []string{user, child, copyPath, nested, guardian}
	mixed := collectSessionFiles(files, sessionWindow{Before: parseMustTime(resourceDate + "59Z").Add(time.Hour)})
	if !reflect.DeepEqual(collected, mixed) || len(mixed.Turns) != 2 || len(mixed.Followups) != 1 {
		t.Fatal("child sessions entered user population")
	}
	got := collectAnalyzeSubagents(files, collected, sessionWindow{})
	if got.LinkedTurns != 3 || got.ExcludedTurns != 1 || got.UnlinkedTurns != 0 {
		t.Fatalf("linkage: %#v", got)
	}
	u1 := got.Tasks["u1"]
	if u1.AgentTurns != 2 || u1.Agents.RecordedTokens.Total != 320 || u1.Agents.EstimatedTokens.KnownTurns != 0 || u1.Agents.Seconds.Total != 12 || u1.Agents.Tools.Total != 2 || u1.Agents.CounterMismatches != 1 {
		t.Fatalf("duplicates, inherited data, or subsets were counted: %#v", u1)
	}
	if u1.Parent.RecordedTokens.Total != 30 || u1.Parent.EstimatedTokens.Total != 0 || u1.Parent.Seconds.Total != 10 || u1.Parent.CounterMismatches != 1 {
		t.Fatalf("parent resources mixed: %#v", u1.Parent)
	}
	if got.Tasks["u2"].Agents.RecordedTokens.Total != 60 || got.Tasks["u2"].Parent.EstimatedTokens.Total != 40 {
		t.Fatalf("reused agent associated with wrong task: %#v", got.Tasks["u2"])
	}
	for _, profile := range u1.Profiles {
		if profile.Name == "copied-model" || profile.Name == "parent-model" {
			t.Fatalf("inherited model attributed: %#v", profile)
		}
	}
}

func TestAnalyzeSubagentsMissingConflictingAndForeignRoots(t *testing.T) {
	for _, tc := range []struct {
		name, contextRoot, requestRoot, parent, fork string
		unlinked, excluded                           int
	}{
		{"missing root", "", "", "root", "", 1, 0},
		{"conflicting roots", "u1", "u2", "root", "", 1, 0},
		{"outside selected user turns", "outside", "outside", "root", "", 1, 0},
		{"missing ancestor", "u1", "u1", "missing", "", 1, 0},
		{"cycle", "u1", "u1", "child", "", 1, 0},
		{"foreign fork history", "u1", "u1", "root", "root", 0, 1},
		{"unmarked foreign history", "u1", "u1", "root", "unmarked-foreign", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			user, collected := effectivenessUser(t, dir)
			path := filepath.Join(dir, "rollout-child.jsonl")
			thread := "child"
			fork := tc.fork
			if tc.fork != "" {
				thread = "root"
			}
			if fork == "unmarked-foreign" {
				fork = ""
			}
			writeJSONL(t, path, sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: tc.parent, Role: "role", Depth: 1}, fork, "", ""),
				event(resourceDate+"03Z", "task_started", "c", nil), rootedContext("04", "c", tc.contextRoot, "m"), rootedRequest("05", thread, "c", tc.requestRoot, "r", 100), event(resourceDate+"06Z", "task_complete", "c", nil))
			got := collectAnalyzeSubagents([]string{user, path}, collected, sessionWindow{})
			if got.LinkedTurns != 0 || got.UnlinkedTurns != tc.unlinked || got.ExcludedTurns != tc.excluded || got.Tasks["u1"].AgentTurns != 0 {
				t.Fatalf("guessed association: %#v", got)
			}
		})
	}
}

func TestAnalyzeSubagentRequestOwnershipConflictsDoNotFallBackOrDoubleCount(t *testing.T) {
	dir := t.TempDir()
	user, collected := effectivenessUser(t, dir)
	path := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, path, sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: "root", Role: "role", Depth: 1}, "", "", ""),
		event(resourceDate+"03Z", "task_started", "c", nil), rootedContext("04", "c", "u1", "m"), rootedRequest("05", "child", "c", "u1", "p1", 100), resourceCounter("06", resourceUsage(100, 0, 10, 0)),
		event(resourceDate+"07Z", "task_complete", "c", map[string]any{"duration_ms": 4000}))
	got := collectAnalyzeSubagents([]string{user, path}, collected, sessionWindow{}).Tasks["u1"]
	for _, resource := range []analyze.EffectivenessResources{got.Parent, got.Agents} {
		if resource.RecordedTokens.KnownTurns != 0 || resource.EstimatedTokens.KnownTurns != 0 || resource.RecordedTokens.MissingTurns != 1 || resource.InvalidRecords != 1 {
			t.Fatalf("conflicting response owner counted: %#v", resource)
		}
	}
}

func TestAnalyzeSubagentModelSwitchesDoNotGuessTimeOrToolAttribution(t *testing.T) {
	dir := t.TempDir()
	user, collected := effectivenessUser(t, dir)
	path := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, path, sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: "root", Role: "custom", Depth: 1}, "", "", ""),
		event(resourceDate+"03Z", "task_started", "c", nil), rootedContext("04", "c", "u1", "m1"), rootedRequest("05", "child", "c", "u1", "r1", 100),
		rootedContext("06", "c", "u1", "m2"), rootedRequest("07", "child", "c", "u1", "r2", 200),
		record(resourceDate+"08Z", "response_item", map[string]any{"type": "function_call", "call_id": "known"}), record(resourceDate+"09Z", "response_item", map[string]any{"type": "function_call"}),
		event(resourceDate+"10Z", "task_complete", "c", map[string]any{"duration_ms": 4000}))
	got := collectAnalyzeSubagents([]string{user, path}, collected, sessionWindow{}).Tasks["u1"]
	if got.Agents.RecordedTokens.Total != 320 || got.Agents.Seconds.Total != 4 || got.Agents.Tools.Total != 1 || got.Agents.Tools.MissingTurns != 1 {
		t.Fatalf("resources or anonymous tools: %#v", got)
	}
	models := map[string]float64{}
	for _, profile := range got.Profiles {
		if profile.Dimension == "model" {
			models[profile.Name] = profile.Resources.RecordedTokens.Total
			if profile.Resources.Seconds.KnownTurns != 0 || profile.Resources.Seconds.MissingTurns != 1 || profile.Resources.Tools.KnownTurns != 0 {
				t.Fatal("model-switch time or tools guessed")
			}
		}
	}
	if !reflect.DeepEqual(models, map[string]float64{"m1": 110, "m2": 210}) {
		t.Fatalf("wrong actual models: %#v", models)
	}
}

func TestAnalyzeSubagentsOldFormatsMissingValuesAndCounterDisagreements(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		records                    []any
		recorded, estimated        float64
		missing, mismatch, invalid int
	}{
		{"legacy cumulative", []any{resourceCounter("05", resourceUsage(100, 80, 10, 4)), resourceCounter("06", resourceUsage(100, 80, 10, 4)), resourceCounter("07", resourceUsage(120, 80, 20, 4))}, 0, 140, 0, 0, 0},
		{"missing", nil, 0, 0, 1, 0, 0},
		{"reset", []any{resourceCounter("05", resourceUsage(100, 0, 0, 0)), resourceCounter("06", resourceUsage(10, 0, 0, 0))}, 0, 0, 1, 0, 1},
		{"record takes priority", []any{rootedRequest("05", "child", "c", "u1", "r", 100), resourceCounter("06", resourceUsage(999, 0, 0, 0))}, 110, 0, 0, 1, 0},
		{"invalid record no fallback", []any{resourceRequest("05", "child", "c", "r", map[string]any{"total_tokens": -1}), resourceCounter("06", resourceUsage(100, 0, 0, 0))}, 0, 0, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			user, collected := effectivenessUser(t, dir)
			path := filepath.Join(dir, "rollout-old.jsonl")
			records := []any{sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: "root", Depth: 1}, "", "", ""), event(resourceDate+"03Z", "task_started", "c", nil), rootedContext("04", "c", "u1", "")}
			records = append(records, tc.records...)
			records = append(records, event(resourceDate+"10Z", "task_complete", "c", nil))
			writeJSONL(t, path, records...)
			got := collectAnalyzeSubagents([]string{user, path}, collected, sessionWindow{}).Tasks["u1"].Agents
			if got.RecordedTokens.Total != tc.recorded || got.EstimatedTokens.Total != tc.estimated || got.RecordedTokens.MissingTurns != tc.missing || got.CounterMismatches != tc.mismatch || got.InvalidRecords != tc.invalid || got.Seconds.KnownTurns != 0 || got.Seconds.MissingTurns != 1 {
				t.Fatalf("resource state: %#v", got)
			}
		})
	}
}

func TestSubagentEffectivenessReportAndHTMLCoverageAndCaveats(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		tr, err := i18n.New(lang)
		if err != nil {
			t.Fatal(err)
		}
		details := &analyze.SubagentEffectivenessDetails{LinkedTurns: 2, UnlinkedTurns: 3, Cohorts: []analyze.SubagentCohort{{TaskType: "bugfix", UsesSubagents: true, Stats: analyze.EffectivenessStats{Samples: 1, Steering: 1}, Agents: analyze.EffectivenessResources{RecordedTokens: analyze.ResourceMetric{Total: 120, Tasks: 1, KnownTurns: 2, MissingTurns: 1}}}}}
		lines := strings.Join(subagentEffectivenessLines(tr, details), "\n")
		for _, key := range []string{"subeffect_difficulty", "subeffect_outcome", "subeffect_sources", "effectiveness_insufficient"} {
			if !strings.Contains(lines, tr.T(key)) {
				t.Fatalf("missing caveat %s", key)
			}
		}
		if !strings.Contains(lines, "token_usage_record=120.0") || !strings.Contains(lines, "token_count="+tr.T("subagent_resource_unknown")) || !strings.Contains(lines, "2/1") {
			t.Fatalf("coverage or source disclosure: %s", lines)
		}
		details.Profiles = []analyze.SubagentCohort{{TaskType: "bugfix", Dimension: "model", Name: "<script>private()</script>", Stats: analyze.EffectivenessStats{Samples: 1}}}
		html, err := RenderHTMLReport(HTMLReportInput{Translator: tr, Effectiveness: analyze.EffectivenessAnalysis{SubagentDetails: details}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, tr.T("subagent_effectiveness")) || strings.Contains(html, "<script>private") || !strings.Contains(html, "&lt;script&gt;") {
			t.Fatal("HTML omitted or failed to escape agent statistics")
		}
	}
}

func TestSubagentEffectivenessCohortThresholdRequiresMatchedTaskTypes(t *testing.T) {
	cohort := analyze.SubagentCohort{TaskType: "bugfix", UsesSubagents: true, Stats: analyze.EffectivenessStats{Samples: 10}}
	for _, tc := range []struct {
		typeName string
		n        int
		want     bool
	}{{"bugfix", 9, false}, {"research", 10, false}, {"bugfix", 10, true}} {
		peers := []analyze.SubagentCohort{cohort, {TaskType: tc.typeName, Stats: analyze.EffectivenessStats{Samples: tc.n}}}
		if got := subagentCohortComparable(cohort, peers); got != tc.want {
			t.Fatalf("matched threshold: %+v got %v", tc, got)
		}
	}
	routing := analyze.SubagentCohort{TaskType: "bugfix", Dimension: "routing", Name: "same / high", UsesSubagents: true, Stats: analyze.EffectivenessStats{Samples: 10}}
	peers := []analyze.SubagentCohort{routing, {TaskType: "bugfix", Dimension: "routing", Name: "same / low", Stats: analyze.EffectivenessStats{Samples: 10}}}
	if subagentCohortComparable(routing, peers) {
		t.Fatal("different parent effort treated as matched routing")
	}
}

func TestAnalyzeSubagentsRespectsWindowAndDoesNotCreateSemanticInputs(t *testing.T) {
	dir := t.TempDir()
	user, collected := effectivenessUser(t, dir)
	child := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, child, sessionMeta("child", resourceDate+"02Z", "subagent", &spawn{ID: "child", Parent: "root", Role: "custom", Depth: 1}, "", "", ""), event(resourceDate+"03Z", "task_started", "c", nil), rootedContext("04", "c", "u1", "m"), rootedRequest("05", "child", "c", "u1", "r", 100))
	before := parseMustTime(resourceDate + "04Z")
	got := collectAnalyzeSubagents([]string{user, child}, collected, sessionWindow{Before: before})
	if got.LinkedTurns != 1 || got.Tasks["u1"].Agents.RecordedTokens.KnownTurns != 0 {
		t.Fatalf("cutoff not applied: %#v", got)
	}
	got = collectAnalyzeSubagents([]string{user, child}, collected, sessionWindow{Since: before, Before: before.Add(time.Hour)})
	if got.LinkedTurns != 0 {
		t.Fatal("agent start before window included")
	}
	mixed := collectSessionFiles([]string{user, child}, sessionWindow{Before: parseMustTime(resourceDate + "59Z").Add(time.Hour)})
	if !reflect.DeepEqual(analyze.BuildSemanticInputs(collected.Interactions, collected.Followups), analyze.BuildSemanticInputs(mixed.Interactions, mixed.Followups)) || len(mixed.Turns) != 2 {
		t.Fatal("child changes semantic calls or user tasks")
	}
}
