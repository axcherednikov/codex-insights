package main

import (
	"time"

	"codex-insights/internal/sessions"
)

type sessionWindow struct {
	Since                   time.Time
	Before                  time.Time
	LegacyExcludeOriginator string
}

type collectedSessions struct {
	Interactions           []sessions.Interaction
	Turns                  []sessions.Turn
	Followups              []sessions.Followup
	UserSessions           int
	ExcludedJudgeSessions  int
	LegacyExcludedSessions int
}

// collectSessions is the single source of truth for analyze and golden export
// session selection. It deliberately preserves the established filtering
// order and silently skips malformed individual rollout files.
func collectSessions(root string, window sessionWindow) (collectedSessions, error) {
	files, err := sessions.FindRollouts(root)
	if err != nil {
		return collectedSessions{}, err
	}
	return collectSessionFiles(files, window), nil
}

func collectSessionFiles(files []string, window sessionWindow) collectedSessions {
	var collected collectedSessions
	for _, file := range files {
		meta, err := sessions.ReadMeta(file)
		if err != nil || meta.ThreadSource != "user" {
			continue
		}
		isJudge, err := sessions.IsInsightsJudgeSession(file)
		if err != nil {
			continue
		}
		if isJudge {
			if timestampInWindow(meta.StartedAt, window.Since, window.Before) {
				collected.ExcludedJudgeSessions++
			}
			continue
		}
		if window.LegacyExcludeOriginator != "" && meta.Originator == window.LegacyExcludeOriginator {
			collected.LegacyExcludedSessions++
			continue
		}
		interactions, err := sessions.ParseInteractions(file, window.Before)
		if err != nil {
			continue
		}
		filtered := filterInteractions(interactions, window.Since, window.Before)
		if len(filtered) == 0 {
			continue
		}
		collected.UserSessions++
		collected.Interactions = append(collected.Interactions, filtered...)
		turns, err := sessions.ParseTurns(file, window.Before)
		if err == nil {
			collected.Turns = append(collected.Turns, filterTurns(turns, window.Since, window.Before)...)
		}
		collected.Followups = append(collected.Followups, sessions.BuildFollowups(filtered)...)
	}
	return collected
}
