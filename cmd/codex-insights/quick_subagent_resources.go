package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/axcherednikov/codex-insights/internal/i18n"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

type subagentUsageTotals struct {
	total    int64
	parts    [4]int64
	missing  [4]bool
	known    bool
	overflow bool
}

type subagentUsageSources struct{ recorded, estimated subagentUsageTotals }

type subagentResources struct {
	subagentUsageSources
	byRole, byModel                                     map[string]*subagentUsageSources
	recordedTurns, estimatedTurns, missingTurns         int
	invalidRecords, invalidCounters, mismatchedCounters int
	unverifiableCounters                                int
	durationMS                                          int64
	durationTurns, missingDuration, invalidDuration     int
	durationOverflow                                    bool
}

func newSubagentResources() subagentResources {
	return subagentResources{byRole: map[string]*subagentUsageSources{}, byModel: map[string]*subagentUsageSources{}}
}

func usageFields(u sessions.SubagentTokenUsage) []*int64 {
	return []*int64{u.InputTokens, u.CachedInputTokens, u.OutputTokens, u.ReasoningOutputTokens}
}

func (s *subagentUsageTotals) add(u sessions.SubagentTokenUsage) {
	s.known = true
	if s.total > (1<<63-1)-*u.TotalTokens {
		s.overflow = true

		return
	}
	s.total += *u.TotalTokens
	for i, field := range usageFields(u) {
		if field == nil {
			s.missing[i] = true
		} else {
			s.parts[i] += *field
		}
	}
}

// Only inclusive totals are added. Cached input and reasoning output are
// retained for disclosure, never added again to the total.
func (s *subagentResources) addUsage(role, model string, u sessions.SubagentTokenUsage, recorded bool) {
	for _, group := range []struct {
		table map[string]*subagentUsageSources
		key   string
	}{{s.byRole, role}, {s.byModel, model}} {
		if group.table[group.key] == nil {
			group.table[group.key] = &subagentUsageSources{}
		}
		row := group.table[group.key]
		if recorded {
			row.recorded.add(u)
		} else {
			row.estimated.add(u)
		}
	}
	if recorded {
		s.recorded.add(u)
	} else {
		s.estimated.add(u)
	}
}

type subagentRequestUsage struct {
	usage   sessions.SubagentTokenUsage
	model   string
	invalid bool
}

func (s *subagentResources) addTurn(childID, turnID, role string, turn *observedTurn, owners map[string]string) {
	s.addDuration(turn)
	legacy, counterState := legacySubagentUsage(turn)
	if counterState == "invalid" {
		s.invalidCounters++
	}
	requests, anonymous := collectSubagentRequests(childID, turnID, turn, owners)
	s.invalidRecords += anonymous
	total := s.addRecordedRequests(role, requests)
	if total.known {
		s.recordedTurns++
		s.compareTokenSources(total, legacy, counterState)

		return
	}
	// Never replace existing but unconfirmed request records with an estimate.
	if len(turn.usageRecords) == 0 && counterState == "valid" {
		s.estimatedTurns++
		s.addUsage(role, subagentSingleModel(turn), legacy, false)

		return
	}
	s.missingTurns++
}

func collectSubagentRequests(childID, turnID string, turn *observedTurn, owners map[string]string) (map[string]*subagentRequestUsage, int) {
	requests := map[string]*subagentRequestUsage{}
	anonymous := map[string]bool{}
	for _, record := range turn.usageRecords {
		if record.ThreadID != childID {
			continue
		}
		if record.ResponseID == "" {
			anonymous[record.Timestamp] = true

			continue
		}
		request := requests[record.ResponseID]
		if request == nil {
			request = &subagentRequestUsage{}
			requests[record.ResponseID] = request
		}
		request.merge(record, turn, owners[record.ResponseID], childID+"\x00"+turnID)
	}

	return requests, len(anonymous)
}

func (request *subagentRequestUsage) merge(record sessions.SubagentUsageRecord, turn *observedTurn, owner, expectedOwner string) {
	usage, valid := record.Usage.Validated()
	at, err := time.Parse(time.RFC3339Nano, record.Timestamp)
	if valid && usageDisagrees(request.usage, usage) {
		request.invalid = true
	}
	if !valid || err != nil || at.Before(turn.start) || owner != expectedOwner {
		request.invalid = true

		return
	}
	model := subagentModelAt(turn, at)
	if request.model != "" && request.model != model {
		model = "unknown"
	}
	combined := mergeUsage(request.usage, usage)
	merged, ok := combined.Validated()
	if !ok {
		request.invalid = true

		return
	}
	request.usage, request.model = merged, model
}

func (s *subagentResources) addRecordedRequests(role string, requests map[string]*subagentRequestUsage) subagentUsageTotals {
	keys := make([]string, 0, len(requests))
	for id := range requests {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	var total subagentUsageTotals
	for _, id := range keys {
		request := requests[id]
		if request.invalid {
			s.invalidRecords++

			continue
		}
		total.add(request.usage)
		s.addUsage(role, request.model, request.usage, true)
	}

	return total
}

func (s *subagentResources) compareTokenSources(total subagentUsageTotals, legacy sessions.SubagentTokenUsage, state string) {
	if state != "valid" || total.overflow {
		s.unverifiableCounters++

		return
	}
	different := total.total != *legacy.TotalTokens
	for i, field := range usageFields(legacy) {
		if field != nil && !total.missing[i] && *field != total.parts[i] {
			different = true
		}
	}
	if different {
		s.mismatchedCounters++
	}
}

func subagentRecordedDuration(turn *observedTurn) (*int64, bool) {
	if turn.status != "complete" {
		return nil, false
	}
	var value *int64
	invalid := false
	for _, terminal := range turn.terminals {
		if terminal.Status != "complete" || !parseMustTime(terminal.Timestamp).Equal(parseMustTime(turn.terminalAt)) {
			continue
		}
		if terminal.InvalidDuration {
			invalid = true
		}
		if terminal.DurationMS == nil {
			continue
		}
		if *terminal.DurationMS < 0 || (value != nil && *value != *terminal.DurationMS) {
			invalid = true
		}
		value = terminal.DurationMS
	}

	return value, invalid
}

func (s *subagentResources) addDuration(turn *observedTurn) {
	value, invalid := subagentRecordedDuration(turn)
	if value == nil || invalid {
		s.missingDuration++
		if invalid {
			s.invalidDuration++
		}

		return
	}
	s.durationTurns++
	if s.durationMS > (1<<63-1)-*value {
		s.durationOverflow = true

		return
	}
	s.durationMS += *value
}

func subagentModelAt(t *observedTurn, at time.Time) string {
	model := "unknown"
	var latest time.Time
	for _, setting := range t.settingHistory {
		ts, err := time.Parse(time.RFC3339Nano, setting.Timestamp)
		if err != nil || ts.Before(t.start) || ts.After(at) {
			continue
		}
		m := setting.Model
		if m == "" {
			m = "unknown"
		}
		if latest.IsZero() || ts.After(latest) {
			latest, model = ts, m
		} else if ts.Equal(latest) && model != m {
			model = "unknown"
		}
	}

	return model
}

func subagentSingleModel(t *observedTurn) string {
	models := map[string]bool{}
	for _, setting := range t.settingHistory {
		ts, err := time.Parse(time.RFC3339Nano, setting.Timestamp)
		if err == nil && !ts.Before(t.start) && setting.Model != "" {
			models[setting.Model] = true
		} else {
			return "unknown"
		}
	}
	if len(models) == 1 {
		for model := range models {
			return model
		}
	}

	return "unknown"
}

func usageDisagrees(a, b sessions.SubagentTokenUsage) bool {
	left, right := append(usageFields(a), a.TotalTokens), append(usageFields(b), b.TotalTokens)
	for i := range left {
		if left[i] != nil && right[i] != nil && *left[i] != *right[i] {
			return true
		}
	}

	return false
}

func mergeUsage(a, b sessions.SubagentTokenUsage) sessions.SubagentTokenUsage {
	if a.InputTokens == nil {
		a.InputTokens = b.InputTokens
	}
	if a.CachedInputTokens == nil {
		a.CachedInputTokens = b.CachedInputTokens
	}
	if a.OutputTokens == nil {
		a.OutputTokens = b.OutputTokens
	}
	if a.ReasoningOutputTokens == nil {
		a.ReasoningOutputTokens = b.ReasoningOutputTokens
	}
	if a.TotalTokens == nil {
		a.TotalTokens = b.TotalTokens
	}

	return a
}

// token_count is cumulative: take the last snapshot minus the starting
// baseline, not the sum of snapshots or repeated last_token_usage values.
func legacySubagentUsage(turn *observedTurn) (sessions.SubagentTokenUsage, string) {
	if turn.counterAmbiguous && len(turn.tokenCounts) > 0 {
		return sessions.SubagentTokenUsage{}, "invalid"
	}
	snapshots, valid := collectSubagentCounterSnapshots(turn)
	if !valid {
		return sessions.SubagentTokenUsage{}, "invalid"
	}
	if len(snapshots) == 0 {
		return sessions.SubagentTokenUsage{}, "missing"
	}
	times := make([]time.Time, 0, len(snapshots))
	for at := range snapshots {
		times = append(times, at)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	last := snapshots[times[0]]
	for _, at := range times[1:] {
		next := snapshots[at]
		if _, ok := next.Delta(&last); !ok {
			return sessions.SubagentTokenUsage{}, "invalid"
		}
		last = next
	}

	return last, "valid"
}

func collectSubagentCounterSnapshots(turn *observedTurn) (map[time.Time]sessions.SubagentTokenUsage, bool) {
	snapshots := map[time.Time]sessions.SubagentTokenUsage{}
	var baseline *sessions.SubagentTokenUsage
	for _, count := range turn.tokenCounts {
		at, err := time.Parse(time.RFC3339Nano, count.Timestamp)
		if err != nil || at.Before(turn.start) {
			continue
		}
		at = at.UTC()
		initial, ok := count.Baseline.Validated()
		if !ok {
			return nil, false
		}
		if baseline != nil && usageDisagrees(*baseline, initial) {
			return nil, false
		}
		baseline = &initial
		delta, ok := count.Total.Delta(count.Baseline)
		if !ok {
			return nil, false
		}
		if previous, exists := snapshots[at]; exists {
			if usageDisagrees(previous, delta) {
				return nil, false
			}
			delta = mergeUsage(previous, delta)
		}
		snapshots[at] = delta
	}

	return snapshots, true
}

func resourceTotal(s subagentUsageTotals, tr i18n.Translator) string {
	if !s.known || s.overflow {
		return tr.T("subagent_resource_unknown")
	}

	return strconv.FormatInt(s.total, 10)
}

func printResourceSource(tr i18n.Translator, label string, u subagentUsageTotals, turns int) {
	fmt.Printf("  %s: %s (%d %s)\n", label, resourceTotal(u, tr), turns, tr.T("subagents_working_short"))
	if !u.known || u.overflow {
		return
	}
	parts := make([]string, len(u.parts))
	for i, n := range u.parts {
		parts[i] = strconv.FormatInt(n, 10)
		if u.missing[i] {
			parts[i] = tr.T("subagent_resource_unknown")
		}
	}
	fmt.Printf("    %s: %s; %s: %s; %s: %s; %s: %s\n", tr.T("subagent_input_tokens"), parts[0], tr.T("subagent_cached_tokens"), parts[1], tr.T("subagent_output_tokens"), parts[2], tr.T("subagent_reasoning_tokens"), parts[3])
}

func printSubagentResources(tr i18n.Translator, s subagentResources) {
	printResourceSource(tr, tr.T("subagent_recorded_tokens"), s.recorded, s.recordedTurns)
	printResourceSource(tr, tr.T("subagent_estimated_tokens"), s.estimated, s.estimatedTurns)
	fmt.Printf("    %s\n", tr.T("subagent_token_subsets_note"))
	for _, table := range []struct {
		label  string
		values map[string]*subagentUsageSources
	}{{"subagent_tokens_by_role", s.byRole}, {"subagent_tokens_by_model", s.byModel}} {
		if len(table.values) == 0 {
			continue
		}
		fmt.Printf("  %s (token_usage_record / token_count):\n", tr.T(table.label))
		keys := make([]string, 0, len(table.values))
		for key := range table.values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			row := table.values[key]
			fmt.Printf("    %s: %s / %s\n", key, resourceTotal(row.recorded, tr), resourceTotal(row.estimated, tr))
		}
	}
	duration := tr.T("subagent_resource_unknown")
	if s.durationTurns > 0 && !s.durationOverflow {
		duration = fmt.Sprintf("%d.%03d %s", s.durationMS/secondsPerMillisecond, s.durationMS%secondsPerMillisecond, tr.T("seconds_short"))
	}
	fmt.Printf("  %s: %s (%d %s)\n", tr.T("subagent_working_time"), duration, s.durationTurns, tr.T("subagents_working_short"))
	fmt.Printf("    %s\n", tr.T("subagent_working_time_note"))
	if s.recorded.overflow || s.estimated.overflow || s.durationOverflow {
		fmt.Printf("  %s\n", tr.T("subagent_resource_overflow"))
	}
	for _, counter := range []struct {
		key   string
		count int
	}{
		{"subagent_missing_tokens", s.missingTurns}, {"subagent_invalid_tokens", s.invalidRecords},
		{"subagent_invalid_counters", s.invalidCounters}, {"subagent_counter_mismatches", s.mismatchedCounters},
		{"subagent_unverifiable_counters", s.unverifiableCounters}, {"subagent_missing_time", s.missingDuration},
		{"subagent_invalid_time", s.invalidDuration},
	} {
		if counter.count > 0 {
			fmt.Printf("  %s: %d\n", tr.T(counter.key), counter.count)
		}
	}
}
