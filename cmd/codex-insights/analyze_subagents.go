package main

import (
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/sessions"
)

// Reuse quick's reconciliation and resource accounting. Only explicit root
// turn identities with a verified ancestry enter user-task cohorts.
func collectAnalyzeSubagents(files []string, collected collectedSessions, window sessionWindow) analyze.SubagentEvidence {
	out := subagentStats{}
	owners := map[string]string{}
	children := out.collectChildFiles(files, window.LegacyExcludeOriginator)
	turns := out.collectChildTurns(children, window.Before, owners)
	parents := readEffectivenessParents(collected, window.Before, owners, &out)
	evidence := analyze.SubagentEvidence{Tasks: map[string]analyze.SubagentTaskEvidence{}}
	addParentEvidence(&evidence, parents, collected, owners, &out)
	for id, child := range children {
		rootSession := subagentRootSession(id, children, collected.SessionFiles)
		creation, err := time.Parse(time.RFC3339Nano, child.meta.StartedAt)
		for turnID, turn := range turns[id] {
			out.resolveTurnTerminals(turn)
			if turn.start.IsZero() || !inQuickWindow(turn.start, window.Since, window.Before) {
				continue
			}
			if !out.includeSubagentTurn(child, turn, creation, err == nil, window.Since, window.Before) || hasForeignTurnUsage(id, turn) {
				evidence.ExcludedTurns++

				continue
			}
			rootTurn := subagentRootTurn(id, turn)
			if rootTurn == "" || rootSession == "" || collected.TurnSessions[rootTurn] != rootSession {
				evidence.UnlinkedTurns++

				continue
			}
			addLinkedSubagent(&evidence, rootTurn, id, turnID, child, turn, owners)
		}
	}
	evidence.ReadErrors = out.readErrors + out.malformed + out.missingIDs

	return evidence
}

func readEffectivenessParents(collected collectedSessions, before time.Time, owners map[string]string, out *subagentStats) map[string]map[string]*observedTurn {
	parents := map[string]map[string]*observedTurn{}
	for id, paths := range collected.SessionFiles {
		child := &observedSubagent{files: map[string]childFileObservation{}, validFiles: map[string]childFileObservation{}}
		for _, path := range paths {
			meta, err := sessions.ReadMeta(path)
			if err != nil || id == "" {
				out.readErrors++

				continue
			}
			child.files[path] = childFileObservation{forked: meta.ForkedFromID != ""}
		}
		parents[id] = out.readChildTurns(id, child, before, owners)
	}

	return parents
}

func addParentEvidence(evidence *analyze.SubagentEvidence, parents map[string]map[string]*observedTurn, collected collectedSessions, owners map[string]string, out *subagentStats) {
	for turnID, sessionID := range collected.TurnSessions {
		evidence.Tasks[turnID] = analyze.SubagentTaskEvidence{Parent: missingEffectivenessResources()}
		turn := parents[sessionID][turnID]
		if turn == nil || sessionID == "" {
			continue
		}
		out.resolveTurnTerminals(turn)
		if turn.terminalConflict || turn.ownershipConflict || (turn.ownershipSeen && !turn.owned) || (turn.ownershipRequired && !turn.ownershipSeen) || hasForeignTurnUsage(sessionID, turn) {
			continue
		}
		resource := newSubagentResources()
		resource.addTurn(sessionID, turnID, "unknown", turn, owners)
		task := evidence.Tasks[turnID]
		task.Parent = effectivenessResources(resource, turn)
		evidence.Tasks[turnID] = task
	}
}

func hasForeignTurnUsage(threadID string, turn *observedTurn) bool {
	for _, record := range turn.usageRecords {
		if record.ThreadID != "" && record.ThreadID != threadID && !parseMustTime(record.Timestamp).Before(turn.start) {
			return true
		}
	}

	return false
}

func missingEffectivenessResources() analyze.EffectivenessResources {
	missing := analyze.ResourceMetric{MissingTurns: 1}

	return analyze.EffectivenessResources{RecordedTokens: missing, EstimatedTokens: missing, Seconds: missing, Tools: missing}
}

func subagentRootSession(id string, children map[string]*observedSubagent, users map[string][]string) string {
	seen := map[string]bool{}
	for !seen[id] {
		seen[id] = true
		child := children[id]
		if child == nil || !child.confirmed || child.attributeConflict || child.meta.ParentID == "" {
			return ""
		}
		id = child.meta.ParentID
		if _, user := users[id]; user {
			return id
		}
	}

	return ""
}

func subagentRootTurn(childID string, turn *observedTurn) string {
	roots := map[string]bool{}
	for _, setting := range turn.settingHistory {
		if setting.RootTurnID != "" && !parseMustTime(setting.Timestamp).Before(turn.start) {
			roots[setting.RootTurnID] = true
		}
	}
	for _, record := range turn.usageRecords {
		if record.ThreadID == childID && record.RootTurnID != "" && !parseMustTime(record.Timestamp).Before(turn.start) {
			roots[record.RootTurnID] = true
		}
	}
	if len(roots) == 1 {
		for id := range roots {
			return id
		}
	}

	return ""
}

func addLinkedSubagent(evidence *analyze.SubagentEvidence, rootTurn, childID, turnID string, child *observedSubagent, turn *observedTurn, owners map[string]string) {
	resource := newSubagentResources()
	role := observedSubagentRole(child)
	resource.addTurn(childID, turnID, role, turn, owners)
	values := effectivenessResources(resource, turn)
	task := evidence.Tasks[rootTurn]
	task.AgentTurns++
	task.Agents.Add(values)
	task.Profiles = append(task.Profiles, analyze.SubagentProfileEvidence{Dimension: "role", Name: role, Resources: values})
	for model, usage := range resource.byModel {
		modelValues := modelEffectivenessResources(model, usage, turn, values)
		task.Profiles = append(task.Profiles, analyze.SubagentProfileEvidence{Dimension: "model", Name: model, Resources: modelValues})
	}
	// Model membership can be confirmed even when token usage is unavailable.
	if model := subagentSingleModel(turn); model != "unknown" && resource.byModel[model] == nil {
		task.Profiles = append(task.Profiles, analyze.SubagentProfileEvidence{Dimension: "model", Name: model, Resources: values})
	}
	evidence.Tasks[rootTurn] = task
	evidence.LinkedTurns++
}

func effectivenessResources(s subagentResources, turn *observedTurn) analyze.EffectivenessResources {
	missing := s.missingTurns
	if missing == 0 && (s.invalidRecords > 0 || s.recorded.overflow || s.estimated.overflow) {
		missing++
	}
	result := analyze.EffectivenessResources{
		RecordedTokens:    usageEffectivenessMetric(s.recorded, s.recordedTurns, missing),
		EstimatedTokens:   usageEffectivenessMetric(s.estimated, s.estimatedTurns, missing),
		Seconds:           analyze.ResourceMetric{Total: float64(s.durationMS) / secondsPerMillisecond, KnownTurns: s.durationTurns, MissingTurns: s.missingDuration},
		Tools:             subagentToolMetric(turn),
		CounterMismatches: s.mismatchedCounters, UnverifiableCounters: s.unverifiableCounters,
		InvalidRecords: s.invalidRecords + s.invalidCounters,
	}
	if s.durationOverflow {
		result.Seconds = analyze.ResourceMetric{MissingTurns: 1}
	}

	return result
}

func usageEffectivenessMetric(usage subagentUsageTotals, turns, missing int) analyze.ResourceMetric {
	if usage.overflow {
		return analyze.ResourceMetric{MissingTurns: missing}
	}

	return analyze.ResourceMetric{Total: float64(usage.total), KnownTurns: turns, MissingTurns: missing}
}

func subagentToolMetric(turn *observedTurn) analyze.ResourceMetric {
	ids := map[string]bool{}
	missing := 0
	for _, call := range turn.toolCalls {
		if parseMustTime(call.Timestamp).Before(turn.start) {
			continue
		}
		if call.ID == "" {
			missing = 1
		} else {
			ids[call.ID] = true
		}
	}

	return analyze.ResourceMetric{Total: float64(len(ids)), KnownTurns: 1, MissingTurns: missing}
}

func modelEffectivenessResources(model string, usage *subagentUsageSources, turn *observedTurn, values analyze.EffectivenessResources) analyze.EffectivenessResources {
	result := analyze.EffectivenessResources{
		RecordedTokens:  usageEffectivenessMetric(usage.recorded, boolCount(usage.recorded.known), 0),
		EstimatedTokens: usageEffectivenessMetric(usage.estimated, boolCount(usage.estimated.known), 0),
	}
	if model == subagentSingleModel(turn) {
		result.Seconds, result.Tools = values.Seconds, values.Tools
	} else {
		result.Seconds.MissingTurns, result.Tools.MissingTurns = 1, 1
	}
	result.RecordedTokens.MissingTurns = values.RecordedTokens.MissingTurns
	result.EstimatedTokens.MissingTurns = values.EstimatedTokens.MissingTurns

	return result
}

func boolCount(value bool) int {
	if value {
		return 1
	}

	return 0
}
