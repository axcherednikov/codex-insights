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
}

func collectQuickSubagents(files []string, since, before time.Time, legacyOriginator string) subagentStats {
	out := subagentStats{roles: map[string]subagentRoleCounts{}, settings: map[string]int{}}
	children := map[string]*observedSubagent{}
	for _, path := range files {
		meta, ok, err := sessions.ReadSubagentMeta(path)
		if err != nil {
			out.readErrors++
			continue
		}
		if !ok {
			continue
		}
		if legacyOriginator != "" {
			ordinary, err := sessions.ReadMeta(path)
			if err != nil {
				out.readErrors++
				continue
			}
			if ordinary.Originator == legacyOriginator {
				continue
			}
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
	turns := map[string]map[string]*observedTurn{}
	for id, child := range children {
		for path, observation := range child.files {
			facts, malformed, err := sessions.ReadSubagentTurns(path, id, before, observation.forked)
			if err != nil {
				out.readErrors++
				continue
			}
			child.validFiles[path] = observation
			out.malformed += malformed
			if turns[id] == nil {
				turns[id] = map[string]*observedTurn{}
			}
			for turnID, fact := range facts {
				t := turns[id][turnID]
				if t == nil {
					t = &observedTurn{status: "incomplete", settings: map[subagentPair]bool{}, terminals: map[string]sessions.SubagentTerminal{}}
					turns[id][turnID] = t
				}
				started, startErr := time.Parse(time.RFC3339Nano, fact.StartedAt)
				if startErr == nil && fact.StartedAt != "" && (t.start.IsZero() || started.Before(t.start)) {
					t.start = started
				}
				for _, terminal := range fact.Terminals {
					t.terminals[terminal.Timestamp+"\x00"+terminal.Status] = terminal
				}
				for _, setting := range fact.Settings {
					m, e := setting.Model, setting.Effort
					if m == "" {
						m = "unknown"
					}
					if e == "" {
						e = "unknown"
					}
					t.settings[subagentPair{m, e}] = true
				}
				if fact.OwnershipSeen {
					if t.ownershipSeen && t.owned != fact.Owned {
						t.ownershipConflict = true
					}
					t.ownershipSeen = true
					t.owned = t.owned || fact.Owned
					t.ownershipConflict = t.ownershipConflict || fact.OwnershipConflict
				}
			}
		}
		reconcileSubagentMeta(child)
		if child.attributeConflict || child.creationConflict {
			out.disputes++
		}
		if child.confirmed {
			if _, err := time.Parse(time.RFC3339Nano, child.meta.StartedAt); err != nil {
				out.disputes++
			}
		}
		if child.confirmed {
			if child.meta.ParentID == "" {
				out.missingParents++
			}
			if !child.meta.HasDepth || child.depthConflict {
				out.missingDepth++
			}
		}
	}
	for _, childTurns := range turns {
		for _, t := range childTurns {
			if t.start.IsZero() {
				out.terminalMismatches += len(t.terminals)
				continue
			}
			for _, terminal := range t.terminals {
				tm, err := time.Parse(time.RFC3339Nano, terminal.Timestamp)
				if err != nil {
					continue
				}
				if tm.Before(t.start) {
					out.terminalMismatches++
					continue
				}
				if t.terminalAt == "" || tm.After(parseMustTime(t.terminalAt)) {
					t.terminalAt, t.status, t.terminalConflict = terminal.Timestamp, terminal.Status, false
				} else if tm.Equal(parseMustTime(t.terminalAt)) && terminal.Status != t.status {
					t.terminalConflict = true
				}
			}
		}
	}
	for id, child := range children {
		creation, creationErr := time.Parse(time.RFC3339Nano, child.meta.StartedAt)
		if child.confirmed && !child.creationConflict && creationErr == nil && inQuickWindow(creation, since, before) {
			out.created++
			role := child.meta.Role
			if child.roleConflict {
				role = "unknown"
			}
			if role == "" {
				role = "unknown"
			}
			r := out.roles[role]
			r.Created++
			out.roles[role] = r
			if child.meta.HasDepth && !child.depthConflict && child.meta.Depth > 1 {
				out.nested++
			}
		}
		for _, t := range turns[id] {
			if t.start.IsZero() {
				continue
			}
			if !inQuickWindow(t.start, since, before) {
				continue
			}
			if child.attributeConflict || child.roleConflict || child.creationConflict {
				continue
			}
			if child.confirmed && creationErr == nil && t.start.Before(creation) {
				out.disputes++
				continue
			}
			if child.meta.ForkedFromID != "" && (!t.ownershipSeen || !t.owned || t.ownershipConflict) {
				out.unattributed++
				continue
			}
			if child.meta.ForkedFromID == "" && t.ownershipSeen && t.ownershipConflict {
				out.disputes++
				continue
			}
			if t.terminalConflict {
				out.disputes++
				continue
			}
			out.working++
			status := t.status
			switch status {
			case "complete":
				out.completed++
			case "aborted":
				out.aborted++
			default:
				status = "incomplete"
				out.incomplete++
			}
			role := child.meta.Role
			if role == "" || child.roleConflict {
				role = "unknown"
			}
			r := out.roles[role]
			r.Working++
			out.roles[role] = r
			if len(t.settings) == 0 {
				out.settings["unknown\tunknown"]++
			} else {
				for setting := range t.settings {
					out.settings[setting.model+"\t"+setting.effort]++
				}
			}
		}
	}
	return out
}

func reconcileSubagentMeta(child *observedSubagent) {
	first := true
	for _, observation := range child.validFiles {
		meta := observation.meta
		if first {
			child.meta = meta
			child.confirmed = true
			first = false
			continue
		}
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
	roles := make([]string, 0, len(stats.roles))
	for role := range stats.roles {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool {
		a, b := stats.roles[roles[i]], stats.roles[roles[j]]
		if a.Created+a.Working != b.Created+b.Working {
			return a.Created+a.Working > b.Created+b.Working
		}
		return roles[i] < roles[j]
	})
	if len(roles) > 0 {
		fmt.Printf("  %s (%s / %s):\n", tr.T("subagents_roles"), tr.T("subagents_created_short"), tr.T("subagents_working_short"))
		for _, role := range roles {
			r := stats.roles[role]
			fmt.Printf("    %s: %d / %d\n", role, r.Created, r.Working)
		}
	}
	keys := make([]string, 0, len(stats.settings))
	for key := range stats.settings {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if stats.settings[keys[i]] != stats.settings[keys[j]] {
			return stats.settings[keys[i]] > stats.settings[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 0 {
		fmt.Printf("  %s (%s):\n", tr.T("subagents_settings"), tr.T("subagents_setting_note"))
		for _, key := range keys {
			fmt.Printf("    %d  %s\n", stats.settings[key], key)
		}
	}
	if stats.unattributed > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_unattributed"), stats.unattributed)
	}
	if stats.missingIDs > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_missing_ids"), stats.missingIDs)
	}
	if stats.missingParents > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_missing_parents"), stats.missingParents)
	}
	if stats.missingDepth > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_missing_depth"), stats.missingDepth)
	}
	if stats.disputes > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_uncertain"), stats.disputes)
	}
	if stats.readErrors > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_read_errors"), stats.readErrors)
	}
	if stats.malformed > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_malformed"), stats.malformed)
	}
	if stats.terminalMismatches > 0 {
		fmt.Printf("  %s: %d\n", tr.T("subagents_terminal_mismatches"), stats.terminalMismatches)
	}
}
