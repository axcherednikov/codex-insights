package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/axcherednikov/codex-insights/internal/i18n"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

type subagentStats struct {
	created, working, completed, aborted, incomplete, nested                     int
	readErrors, missingIDs, missingParents, missingDepth, disputes, unattributed int
	malformed, terminalMismatches                                                int
	roles                                                                        map[string]subagentRoleCounts
	settings                                                                     map[string]int
	resources                                                                    subagentResources
}
type subagentRoleCounts struct{ Created, Working int }
type subagentPair struct{ model, effort string }
type childFileObservation struct {
	meta   sessions.SubagentMeta
	forked bool
}
type observedSubagent struct {
	files             map[string]childFileObservation
	validFiles        map[string]childFileObservation
	meta              sessions.SubagentMeta
	creationConflict  bool
	attributeConflict bool
	roleConflict      bool
	depthConflict     bool
	forkConflict      bool
	confirmed         bool
}
type observedTurn struct {
	start                                   time.Time
	status, terminalAt                      string
	terminalConflict                        bool
	settings                                map[subagentPair]bool
	terminals                               map[string]sessions.SubagentTerminal
	owned, ownershipSeen, ownershipConflict bool
	usageRecords                            []sessions.SubagentUsageRecord
	tokenCounts                             []sessions.SubagentTokenCount
	settingHistory                          []sessions.SubagentSetting
	counterAmbiguous                        bool
	toolCalls                               []sessions.SubagentToolCall
	ownershipRequired                       bool
}

func collectQuickSubagents(files []string, since, before time.Time, legacyOriginator string) subagentStats {
	out := subagentStats{roles: map[string]subagentRoleCounts{}, settings: map[string]int{}, resources: newSubagentResources()}
	owners := map[string]string{}
	children := out.collectChildFiles(files, legacyOriginator)
	turns := out.collectChildTurns(children, before, owners)
	for _, childTurns := range turns {
		for _, turn := range childTurns {
			out.resolveTurnTerminals(turn)
		}
	}
	for id, child := range children {
		creation, err := time.Parse(time.RFC3339Nano, child.meta.StartedAt)
		out.countChildCreation(child, creation, err == nil, since, before)
		for turnID, turn := range turns[id] {
			if out.includeSubagentTurn(child, turn, creation, err == nil, since, before) {
				out.countWorkingSubagentTurn(id, turnID, child, turn, owners)
			}
		}
	}

	return out
}

func readQuickSubagentMeta(path, legacyOriginator string) (sessions.SubagentMeta, bool, error) {
	meta, ok, err := sessions.ReadSubagentMeta(path)
	if err != nil {
		return sessions.SubagentMeta{}, false, fmt.Errorf("read subagent metadata: %w", err)
	}
	if !ok || legacyOriginator == "" {
		return meta, ok, nil
	}
	ordinary, err := sessions.ReadMeta(path)
	if err != nil {
		return sessions.SubagentMeta{}, false, fmt.Errorf("read originator metadata: %w", err)
	}

	return meta, ordinary.Originator != legacyOriginator, nil
}

func (out *subagentStats) collectChildFiles(files []string, legacyOriginator string) map[string]*observedSubagent {
	children := map[string]*observedSubagent{}
	for _, path := range files {
		meta, ok, err := readQuickSubagentMeta(path, legacyOriginator)
		if err != nil {
			out.readErrors++

			continue
		}
		if !ok {
			continue
		}
		if meta.ID == "" {
			out.missingIDs++

			continue
		}
		child := children[meta.ID]
		if child == nil {
			child = &observedSubagent{files: map[string]childFileObservation{}, validFiles: map[string]childFileObservation{}}
			children[meta.ID] = child
		}
		child.files[path] = childFileObservation{meta: meta, forked: meta.ForkedFromID != ""}
	}

	return children
}

func (out *subagentStats) collectChildTurns(children map[string]*observedSubagent, before time.Time, owners map[string]string) map[string]map[string]*observedTurn {
	turns := map[string]map[string]*observedTurn{}
	for id, child := range children {
		turns[id] = out.readChildTurns(id, child, before, owners)
		reconcileSubagentMeta(child)
		out.noteChildMetaCoverage(child)
	}

	return turns
}

func (out *subagentStats) readChildTurns(id string, child *observedSubagent, before time.Time, owners map[string]string) map[string]*observedTurn {
	turns := map[string]*observedTurn{}
	for path, observation := range child.files {
		facts, malformed, err := sessions.ReadSubagentTurns(path, id, before, observation.forked)
		if err != nil {
			out.readErrors++

			continue
		}
		child.validFiles[path] = observation
		out.malformed += malformed
		for turnID, fact := range facts {
			turn := turns[turnID]
			if turn == nil {
				turn = &observedTurn{status: "incomplete", settings: map[subagentPair]bool{}, terminals: map[string]sessions.SubagentTerminal{}}
				turns[turnID] = turn
			}
			mergeSubagentTurn(turn, fact)
			turn.ownershipRequired = turn.ownershipRequired || observation.forked
			recordSubagentResponseOwners(id, turnID, fact.UsageRecords, owners)
		}
	}

	return turns
}

func mergeSubagentTurn(turn *observedTurn, fact sessions.SubagentTurn) {
	started, err := time.Parse(time.RFC3339Nano, fact.StartedAt)
	if err == nil && fact.StartedAt != "" && (turn.start.IsZero() || started.Before(turn.start)) {
		turn.start = started
	}
	for _, terminal := range fact.Terminals {
		turn.terminals[sessions.SubagentTerminalKey(terminal)] = terminal
	}
	turn.usageRecords = append(turn.usageRecords, fact.UsageRecords...)
	turn.tokenCounts = append(turn.tokenCounts, fact.TokenCounts...)
	turn.settingHistory = append(turn.settingHistory, fact.Settings...)
	turn.toolCalls = append(turn.toolCalls, fact.ToolCalls...)
	turn.counterAmbiguous = turn.counterAmbiguous || fact.CounterAmbiguous
	mergeSubagentSettings(turn, fact.Settings)
	if fact.OwnershipSeen {
		if turn.ownershipSeen && turn.owned != fact.Owned {
			turn.ownershipConflict = true
		}
		turn.ownershipSeen = true
		turn.owned = turn.owned || fact.Owned
		turn.ownershipConflict = turn.ownershipConflict || fact.OwnershipConflict
	}
}

func mergeSubagentSettings(turn *observedTurn, settings []sessions.SubagentSetting) {
	for _, setting := range settings {
		model, effort := setting.Model, setting.Effort
		if model == "" {
			model = "unknown"
		}
		if effort == "" {
			effort = "unknown"
		}
		turn.settings[subagentPair{model, effort}] = true
	}
}

func recordSubagentResponseOwners(id, turnID string, usages []sessions.SubagentUsageRecord, owners map[string]string) {
	for _, usage := range usages {
		if usage.ThreadID != id || usage.ResponseID == "" {
			continue
		}
		owner := id + "\x00" + turnID
		if old, exists := owners[usage.ResponseID]; exists && old != owner {
			owners[usage.ResponseID] = ""
		} else if !exists {
			owners[usage.ResponseID] = owner
		}
	}
}

func (out *subagentStats) noteChildMetaCoverage(child *observedSubagent) {
	if child.attributeConflict || child.creationConflict {
		out.disputes++
	}
	if !child.confirmed {
		return
	}
	if _, err := time.Parse(time.RFC3339Nano, child.meta.StartedAt); err != nil {
		out.disputes++
	}
	if child.meta.ParentID == "" {
		out.missingParents++
	}
	if !child.meta.HasDepth || child.depthConflict {
		out.missingDepth++
	}
}

func (out *subagentStats) resolveTurnTerminals(turn *observedTurn) {
	mismatches := map[string]bool{}
	for _, terminal := range turn.terminals {
		at, err := time.Parse(time.RFC3339Nano, terminal.Timestamp)
		if err != nil {
			continue
		}
		if turn.start.IsZero() || at.Before(turn.start) {
			key := at.UTC().Format(time.RFC3339Nano) + "\x00" + terminal.Status
			if !mismatches[key] {
				out.terminalMismatches++
				mismatches[key] = true
			}

			continue
		}
		if turn.terminalAt == "" || at.After(parseMustTime(turn.terminalAt)) {
			turn.terminalAt, turn.status, turn.terminalConflict = terminal.Timestamp, terminal.Status, false
		} else if at.Equal(parseMustTime(turn.terminalAt)) && terminal.Status != turn.status {
			turn.terminalConflict = true
		}
	}
}

func (out *subagentStats) countChildCreation(child *observedSubagent, creation time.Time, valid bool, since, before time.Time) {
	if !child.confirmed || child.creationConflict || !valid || !inQuickWindow(creation, since, before) {
		return
	}
	out.created++
	role := observedSubagentRole(child)
	counts := out.roles[role]
	counts.Created++
	out.roles[role] = counts
	if child.meta.HasDepth && !child.depthConflict && child.meta.Depth > 1 {
		out.nested++
	}
}

func observedSubagentRole(child *observedSubagent) string {
	if child.meta.Role == "" || child.roleConflict {
		return "unknown"
	}

	return child.meta.Role
}

func (out *subagentStats) includeSubagentTurn(child *observedSubagent, turn *observedTurn, creation time.Time, validCreation bool, since, before time.Time) bool {
	if turn.start.IsZero() || !inQuickWindow(turn.start, since, before) {
		return false
	}
	if child.attributeConflict || child.roleConflict || child.creationConflict {
		return false
	}
	if child.confirmed && validCreation && turn.start.Before(creation) {
		out.disputes++

		return false
	}
	if !out.hasSubagentOwnership(child, turn) {
		return false
	}
	if turn.terminalConflict {
		out.disputes++

		return false
	}

	return true
}

func (out *subagentStats) hasSubagentOwnership(child *observedSubagent, turn *observedTurn) bool {
	if child.meta.ForkedFromID != "" {
		if !turn.ownershipSeen || !turn.owned || turn.ownershipConflict {
			out.unattributed++

			return false
		}
	} else if turn.ownershipSeen && turn.ownershipConflict {
		out.disputes++

		return false
	}

	return true
}

func (out *subagentStats) countWorkingSubagentTurn(id, turnID string, child *observedSubagent, turn *observedTurn, owners map[string]string) {
	out.working++
	switch turn.status {
	case "complete":
		out.completed++
	case "aborted":
		out.aborted++
	default:
		out.incomplete++
	}
	role := observedSubagentRole(child)
	counts := out.roles[role]
	counts.Working++
	out.roles[role] = counts
	out.resources.addTurn(id, turnID, role, turn, owners)
	if len(turn.settings) == 0 {
		out.settings["unknown\tunknown"]++
	} else {
		for setting := range turn.settings {
			out.settings[setting.model+"\t"+setting.effort]++
		}
	}
}

func reconcileSubagentMeta(child *observedSubagent) {
	first := true
	for _, observation := range child.validFiles {
		if first {
			child.meta = observation.meta
			child.confirmed = true
			first = false

			continue
		}
		child.compareMeta(observation.meta)
	}
	if child.roleConflict || child.depthConflict || child.forkConflict {
		child.attributeConflict = true
	}
	if child.creationConflict {
		child.attributeConflict = true
	}
	if child.forkConflict {
		child.meta.ForkedFromID = ""
	}
	if child.depthConflict {
		child.meta.HasDepth = false
	}
}

func (child *observedSubagent) compareMeta(meta sessions.SubagentMeta) {
	if child.meta.StartedAt != meta.StartedAt {
		child.creationConflict = true
	}
	if child.meta.Role != meta.Role {
		child.roleConflict = true
	}
	if child.meta.Depth != meta.Depth || child.meta.HasDepth != meta.HasDepth {
		child.depthConflict = true
	}
	if child.meta.ForkedFromID != meta.ForkedFromID {
		child.forkConflict = true
	}
	if child.meta.ParentID != meta.ParentID {
		child.attributeConflict = true
		child.meta.ParentID = ""
	}
}

func parseMustTime(raw string) time.Time { t, _ := time.Parse(time.RFC3339Nano, raw); return t }
func inQuickWindow(ts, since, before time.Time) bool {
	return (before.IsZero() || !ts.After(before)) && (since.IsZero() || !ts.Before(since))
}

func printQuickSubagents(tr i18n.Translator, stats subagentStats) {
	fmt.Println()
	fmt.Printf("%s:\n", tr.T("subagents"))
	fmt.Printf("  %s: %d\n", tr.T("subagents_created"), stats.created)
	fmt.Printf("  %s: %d\n", tr.T("subagents_working_turns"), stats.working)
	fmt.Printf("  %s: %d\n", tr.T("subagents_completed"), stats.completed)
	fmt.Printf("  %s: %d\n", tr.T("subagents_aborted"), stats.aborted)
	fmt.Printf("  %s: %d\n", tr.T("subagents_incomplete"), stats.incomplete)
	fmt.Printf("  %s: %d\n", tr.T("subagents_nested"), stats.nested)
	printQuickSubagentRoles(tr, stats.roles)
	printQuickSubagentSettings(tr, stats.settings)
	printQuickSubagentCoverage(tr, stats)
	printSubagentResources(tr, stats.resources)
}

func printQuickSubagentRoles(tr i18n.Translator, counts map[string]subagentRoleCounts) {
	roles := make([]string, 0, len(counts))
	for role := range counts {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool {
		a, b := counts[roles[i]], counts[roles[j]]
		if a.Created+a.Working != b.Created+b.Working {
			return a.Created+a.Working > b.Created+b.Working
		}

		return roles[i] < roles[j]
	})
	if len(roles) == 0 {
		return
	}
	fmt.Printf("  %s (%s / %s):\n", tr.T("subagents_roles"), tr.T("subagents_created_short"), tr.T("subagents_working_short"))
	for _, role := range roles {
		counts := counts[role]
		fmt.Printf("    %s: %d / %d\n", role, counts.Created, counts.Working)
	}
}

func printQuickSubagentSettings(tr i18n.Translator, settings map[string]int) {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if settings[keys[i]] != settings[keys[j]] {
			return settings[keys[i]] > settings[keys[j]]
		}

		return keys[i] < keys[j]
	})
	if len(keys) == 0 {
		return
	}
	fmt.Printf("  %s (%s):\n", tr.T("subagents_settings"), tr.T("subagents_setting_note"))
	for _, key := range keys {
		fmt.Printf("    %d  %s\n", settings[key], key)
	}
}

func printQuickSubagentCoverage(tr i18n.Translator, stats subagentStats) {
	for _, counter := range []struct {
		key   string
		count int
	}{
		{"subagents_unattributed", stats.unattributed}, {"subagents_missing_ids", stats.missingIDs},
		{"subagents_missing_parents", stats.missingParents}, {"subagents_missing_depth", stats.missingDepth},
		{"subagents_uncertain", stats.disputes}, {"subagents_read_errors", stats.readErrors},
		{"subagents_malformed", stats.malformed}, {"subagents_terminal_mismatches", stats.terminalMismatches},
	} {
		if counter.count > 0 {
			fmt.Printf("  %s: %d\n", tr.T(counter.key), counter.count)
		}
	}
}
