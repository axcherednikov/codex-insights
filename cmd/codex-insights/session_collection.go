package main

import (
	"fmt"
	"time"

	"github.com/axcherednikov/codex-insights/internal/sessions"
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
	TurnSessions           map[string]string
	SessionFiles           map[string][]string
}

// collectSessions is the single source of truth for analyze and golden export
// session selection. It deliberately preserves the established filtering
// order and silently skips malformed individual rollout files.
func collectSessions(root string, window sessionWindow) (collectedSessions, error) {
	files, err := sessions.FindRollouts(root)
	if err != nil {
		return collectedSessions{}, fmt.Errorf("find session rollouts: %w", err)
	}

	return collectSessionFiles(files, window), nil
}

func collectSessionFiles(files []string, window sessionWindow) collectedSessions {
	collected := collectedSessions{TurnSessions: map[string]string{}, SessionFiles: map[string][]string{}}
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
		collected.SessionFiles[meta.ID] = append(collected.SessionFiles[meta.ID], file)
		collected.Interactions = append(collected.Interactions, filtered...)
		turns, err := sessions.ParseTurns(file, window.Before)
		if err == nil {
			selected := filterTurns(turns, window.Since, window.Before)
			collected.Turns = append(collected.Turns, selected...)
			collected.recordTurnSessions(selected, meta.ID)
		}
		collected.Followups = append(collected.Followups, sessions.BuildFollowups(filtered)...)
	}

	return collected
}

func (collected *collectedSessions) recordTurnSessions(turns []sessions.Turn, sessionID string) {
	for _, turn := range turns {
		if previous, exists := collected.TurnSessions[turn.ID]; exists && previous != sessionID {
			collected.TurnSessions[turn.ID] = ""
		} else if !exists {
			collected.TurnSessions[turn.ID] = sessionID
		}
	}
}
