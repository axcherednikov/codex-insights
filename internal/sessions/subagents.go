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
	UsageRecords      []SubagentUsageRecord
	TokenCounts       []SubagentTokenCount
	CounterAmbiguous  bool
}

type SubagentTerminal struct {
	Timestamp, Status string
	DurationMS        *int64
	InvalidDuration   bool
}
type SubagentSetting struct{ Timestamp, Model, Effort string }

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

type subagentTurnParser struct {
	childID                                                     string
	before                                                      time.Time
	forked                                                      bool
	turns                                                       map[string]SubagentTurn
	starts                                                      map[string]time.Time
	owned, ownershipSeen, ownershipConflict, ownershipValueSeen map[string]bool
	malformed                                                   int
	terminalSeen                                                map[string]bool
	currentID                                                   string
	lastCounter                                                 *SubagentTokenUsage
	baselines                                                   map[string]*SubagentTokenUsage
}

type subagentEventPayload struct {
	Type     string          `json:"type"`
	TurnID   string          `json:"turn_id"`
	Duration json.RawMessage `json:"duration_ms"`
	Info     json.RawMessage `json:"info"`
}

// ReadSubagentTurns collects lightweight lifecycle/settings facts for a child rollout.
func ReadSubagentTurns(path, childID string, before time.Time, forked bool) (map[string]SubagentTurn, int, error) {
	parser := newSubagentTurnParser(childID, before, forked)
	if err := fileio.Read(path, parser.scan); err != nil {
		return nil, 0, fmt.Errorf("parse subagent records from %q: %w", path, err)
	}

	return parser.finish(), parser.malformed, nil
}

func newSubagentTurnParser(childID string, before time.Time, forked bool) *subagentTurnParser {
	zero := int64(0)

	return &subagentTurnParser{
		childID: childID, before: before, forked: forked,
		turns: map[string]SubagentTurn{}, starts: map[string]time.Time{},
		owned: map[string]bool{}, ownershipSeen: map[string]bool{}, ownershipConflict: map[string]bool{}, ownershipValueSeen: map[string]bool{},
		terminalSeen: map[string]bool{}, baselines: map[string]*SubagentTokenUsage{},
		lastCounter: &SubagentTokenUsage{InputTokens: &zero, CachedInputTokens: &zero, OutputTokens: &zero, ReasoningOutputTokens: &zero, TotalTokens: &zero},
	}
}

func (p *subagentTurnParser) scan(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, scannerInitialCapacity), subagentMaxLineSize)
	for scanner.Scan() {
		p.parseLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan subagent records: %w", err)
	}

	return nil
}

func (p *subagentTurnParser) parseLine(line []byte) {
	var rec rawRecord
	if json.Unmarshal(line, &rec) != nil {
		p.malformed++

		return
	}
	at, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if err != nil {
		p.malformed++

		return
	}
	if !p.before.IsZero() && at.After(p.before) {
		return
	}
	switch rec.Type {
	case "event_msg":
		p.parseEvent(rec.Payload, at, rec.Timestamp)
	case "turn_context":
		p.parseContext(rec.Payload, rec.Timestamp)
	case "token_usage_record":
		p.parseUsage(rec.Payload, rec.Timestamp)
	case "compacted":
		p.lastCounter = nil
	}
}

func isSubagentLifecycleEvent(kind string) bool {
	switch kind {
	case "token_count", "task_started", "task_complete", "turn_aborted":
		return true
	default:
		return false
	}
}

func (p *subagentTurnParser) parseEvent(data json.RawMessage, at time.Time, timestamp string) {
	var kind struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &kind) != nil {
		p.malformed++

		return
	}
	if !isSubagentLifecycleEvent(kind.Type) {
		return
	}
	var event subagentEventPayload
	if json.Unmarshal(data, &event) != nil {
		p.malformed++

		return
	}
	if event.Type == "token_count" {
		p.parseCounter(event, timestamp)

		return
	}
	if event.TurnID == "" {
		p.malformed++

		return
	}
	switch event.Type {
	case "task_started":
		p.start(event.TurnID, at, timestamp)
	case "task_complete", "turn_aborted":
		p.complete(event, at, timestamp)
	}
}

func (p *subagentTurnParser) parseCounter(event subagentEventPayload, timestamp string) {
	var info struct {
		Total json.RawMessage `json:"total_token_usage"`
	}
	if json.Unmarshal(event.Info, &info) != nil || len(info.Total) == 0 || string(info.Total) == "null" {
		return
	}
	var usage SubagentTokenUsage
	var total *SubagentTokenUsage
	if json.Unmarshal(info.Total, &usage) == nil {
		total = &usage
	}
	if p.currentID != "" && !p.forked {
		turn := p.turns[p.currentID]
		attributed := total
		if event.TurnID != "" && event.TurnID != p.currentID {
			attributed = nil
		}
		turn.TokenCounts = append(turn.TokenCounts, SubagentTokenCount{Timestamp: timestamp, Baseline: p.baselines[p.currentID], Total: attributed})
		p.turns[p.currentID] = turn
	}
	p.lastCounter = total
}

func (p *subagentTurnParser) start(id string, at time.Time, timestamp string) {
	turn := p.turns[id]
	if p.currentID != "" && p.currentID != id {
		previous := p.turns[p.currentID]
		previous.CounterAmbiguous = true
		p.turns[p.currentID] = previous
		turn.CounterAmbiguous = true
		p.lastCounter = nil
	}
	if _, exists := p.baselines[id]; !exists {
		p.baselines[id] = p.lastCounter
	}
	p.currentID = id
	if old, exists := p.starts[id]; !exists || at.Before(old) {
		p.starts[id], turn.StartedAt = at, timestamp
	}
	p.turns[id] = turn
}

func decodeSubagentTerminal(event subagentEventPayload, timestamp string) SubagentTerminal {
	terminal := SubagentTerminal{Timestamp: timestamp, Status: "complete"}
	if event.Type == "turn_aborted" {
		terminal.Status = "aborted"
	}
	if len(event.Duration) > 0 && string(event.Duration) != "null" {
		var duration int64
		if json.Unmarshal(event.Duration, &duration) != nil {
			terminal.InvalidDuration = true
		} else {
			terminal.DurationMS = &duration
		}
	}

	return terminal
}

func (p *subagentTurnParser) complete(event subagentEventPayload, at time.Time, timestamp string) {
	turn := p.turns[event.TurnID]
	terminal := decodeSubagentTerminal(event, timestamp)
	key := event.TurnID + "\x00" + SubagentTerminalKey(terminal)
	if !p.terminalSeen[key] {
		turn.Terminals = append(turn.Terminals, terminal)
		p.terminalSeen[key] = true
	}
	if p.currentID == event.TurnID && !at.Before(p.starts[event.TurnID]) {
		p.currentID = ""
	}
	p.turns[event.TurnID] = turn
}

func (p *subagentTurnParser) parseContext(data json.RawMessage, timestamp string) {
	var context struct {
		TurnID string `json:"turn_id"`
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if json.Unmarshal(data, &context) != nil || context.TurnID == "" {
		p.malformed++

		return
	}
	turn := p.turns[context.TurnID]
	turn.Settings = append(turn.Settings, SubagentSetting{Timestamp: timestamp, Model: context.Model, Effort: context.Effort})
	p.turns[context.TurnID] = turn
}

func (p *subagentTurnParser) parseUsage(data json.RawMessage, timestamp string) {
	var record struct {
		ThreadID   string          `json:"thread_id"`
		TurnID     string          `json:"turn_id"`
		ResponseID string          `json:"response_id"`
		Usage      json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &record) != nil || record.TurnID == "" {
		p.malformed++

		return
	}
	var usage SubagentTokenUsage
	var usageValue *SubagentTokenUsage
	if json.Unmarshal(record.Usage, &usage) == nil {
		usageValue = &usage
	}
	turn := p.turns[record.TurnID]
	turn.UsageRecords = append(turn.UsageRecords, SubagentUsageRecord{ThreadID: record.ThreadID, ResponseID: record.ResponseID, Timestamp: timestamp, Usage: usageValue})
	p.turns[record.TurnID] = turn
	if p.forked {
		p.recordOwnership(record.TurnID, record.ThreadID == p.childID)
	}
}

func (p *subagentTurnParser) recordOwnership(id string, owned bool) {
	if p.ownershipValueSeen[id] && p.owned[id] != owned {
		p.ownershipConflict[id] = true
	}
	p.ownershipValueSeen[id], p.ownershipSeen[id], p.owned[id] = true, true, owned
}

func (p *subagentTurnParser) finish() map[string]SubagentTurn {
	for id, turn := range p.turns {
		turn.Owned, turn.OwnershipSeen, turn.OwnershipConflict = p.owned[id], p.ownershipSeen[id], p.ownershipConflict[id]
		p.turns[id] = turn
	}

	return p.turns
}
