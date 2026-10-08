package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/axcherednikov/codex-insights/internal/fileio"
)

const subagentMaxLineSize = 16 * 1024 * 1024

// SubagentMeta is the logged identity and spawn context for an ordinary child rollout.
type SubagentMeta struct {
	ID, ParentID, Role string
	Depth              int
	HasDepth           bool
	StartedAt          string
	ForkedFromID       string
}

// SubagentTurn contains lifecycle and execution-setting observations, never conversation content.
type SubagentTurn struct {
	ID, StartedAt     string
	Terminals         []SubagentTerminal
	Settings          []SubagentSetting
	Owned             bool
	OwnershipSeen     bool
	OwnershipConflict bool
}

type SubagentTerminal struct{ Timestamp, Status string }
type SubagentSetting struct{ Model, Effort string }

// ReadSubagentMeta reads only rollout metadata and classifies solely on the logged spawn structure.
func ReadSubagentMeta(path string) (SubagentMeta, bool, error) {
	meta, err := ReadMeta(path)
	if err != nil {
		return SubagentMeta{}, false, err
	}
	if meta.ThreadSource != "subagent" {
		return SubagentMeta{}, false, nil
	}
	var source struct {
		Subagent struct {
			ThreadSpawn *struct {
				ID             string `json:"id"`
				ParentThreadID string `json:"parent_thread_id"`
				Depth          *int   `json:"depth"`
				AgentRole      string `json:"agent_role"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	}
	b, err := json.Marshal(meta.Source)
	if err != nil || json.Unmarshal(b, &source) != nil || source.Subagent.ThreadSpawn == nil {
		return SubagentMeta{}, false, nil
	}
	spawn := source.Subagent.ThreadSpawn
	out := SubagentMeta{ID: meta.ID, ParentID: spawn.ParentThreadID, Role: spawn.AgentRole, StartedAt: meta.StartedAt, ForkedFromID: meta.ForkedFromID}
	if spawn.ID != "" && out.ID != "" && spawn.ID != out.ID {
		return SubagentMeta{}, false, nil
	}
	if spawn.Depth != nil {
		out.Depth, out.HasDepth = *spawn.Depth, true
	}
	return out, true, nil
}

// ReadSubagentTurns collects lightweight lifecycle/settings facts for a child rollout.
func ReadSubagentTurns(path, childID string, before time.Time, forked bool) (map[string]SubagentTurn, int, error) {
	turns := make(map[string]SubagentTurn)
	starts := make(map[string]time.Time)
	owned := make(map[string]bool)
	ownershipSeen := make(map[string]bool)
	ownershipConflict := make(map[string]bool)
	ownershipValueSeen := make(map[string]bool)
	malformed := 0
	terminalSeen := make(map[string]bool)
	err := fileio.Read(path, func(r io.Reader) error {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, scannerInitialCapacity), subagentMaxLineSize)
		for s.Scan() {
			var rec struct {
				Timestamp string          `json:"timestamp"`
				Type      string          `json:"type"`
				Payload   json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(s.Bytes(), &rec) != nil {
				malformed++
				continue
			}
			ts, e := time.Parse(time.RFC3339Nano, rec.Timestamp)
			if e != nil || (!before.IsZero() && ts.After(before)) {
				if e != nil {
					malformed++
				}
				continue
			}
			switch rec.Type {
			case "event_msg":
				var p struct {
					Type   string `json:"type"`
					TurnID string `json:"turn_id"`
				}
				if json.Unmarshal(rec.Payload, &p) != nil {
					malformed++
					continue
				}
				if p.Type != "task_started" && p.Type != "task_complete" && p.Type != "turn_aborted" {
					continue
				}
				if p.TurnID == "" {
					malformed++
					continue
				}
				t := turns[p.TurnID]
				switch p.Type {
				case "task_started":
					if old, ok := starts[p.TurnID]; !ok || ts.Before(old) {
						starts[p.TurnID], t.StartedAt = ts, rec.Timestamp
					}
				case "task_complete", "turn_aborted":
					status := "complete"
					if p.Type == "turn_aborted" {
						status = "aborted"
					}
					key := rec.Timestamp + "\x00" + status
					if !terminalSeen[p.TurnID+"\x00"+key] {
						t.Terminals = append(t.Terminals, SubagentTerminal{Timestamp: rec.Timestamp, Status: status})
						terminalSeen[p.TurnID+"\x00"+key] = true
					}
				}
				turns[p.TurnID] = t
			case "turn_context":
				var p struct {
					TurnID string `json:"turn_id"`
					Model  string `json:"model"`
					Effort string `json:"effort"`
				}
				if json.Unmarshal(rec.Payload, &p) == nil && p.TurnID != "" {
					t := turns[p.TurnID]
					t.Settings = append(t.Settings, SubagentSetting{p.Model, p.Effort})
					turns[p.TurnID] = t
				} else {
					malformed++
				}
			case "token_usage_record":
				if !forked {
					continue
				}
				var p struct {
					ThreadID string `json:"thread_id"`
					TurnID   string `json:"turn_id"`
				}
				if json.Unmarshal(rec.Payload, &p) != nil || p.TurnID == "" {
					malformed++
					continue
				}
				isOwned := p.ThreadID == childID
				if ownershipValueSeen[p.TurnID] && owned[p.TurnID] != isOwned {
					ownershipConflict[p.TurnID] = true
				}
				ownershipValueSeen[p.TurnID] = true
				ownershipSeen[p.TurnID] = true
				owned[p.TurnID] = isOwned
			}
		}
		if err := s.Err(); err != nil {
			return fmt.Errorf("scan subagent records: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("parse subagent records from %q: %w", path, err)
	}
	for id, t := range turns {
		t.Owned, t.OwnershipSeen, t.OwnershipConflict = owned[id], ownershipSeen[id], ownershipConflict[id]
		turns[id] = t
	}
	return turns, malformed, nil
}
