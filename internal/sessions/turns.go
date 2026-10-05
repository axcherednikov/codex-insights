package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/fileio"
)

const turnMaxLineSize = 16 * 1024 * 1024

type Turn struct {
	ID            string
	Status        string
	Model         string
	Effort        string
	StartedAt     string
	Tokens        int64
	DurationMS    int64
	TTFTMS        int64
	ToolCalls     int
	UsesSubagents bool
}

type rawRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type eventPayload struct {
	Type       string `json:"type"`
	TurnID     string `json:"turn_id"`
	DurationMS int64  `json:"duration_ms"`
	TTFTMS     int64  `json:"time_to_first_token_ms"`
	Info       *struct {
		TotalTokenUsage struct {
			TotalTokens int64 `json:"total_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

type turnContextPayload struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

type responsePayload struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	ToolName string `json:"tool_name"`
}

type turnParser struct {
	before     time.Time
	turns      []Turn
	current    *Turn
	startTotal int64
	lastTotal  int64
}

func ParseTurns(path string, before time.Time) ([]Turn, error) {
	parser := turnParser{before: before}
	if err := fileio.Read(path, parser.scan); err != nil {
		return nil, fmt.Errorf("parse turns from %q: %w", path, err)
	}

	return parser.finish(), nil
}

func (p *turnParser) scan(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, scannerInitialCapacity), turnMaxLineSize)
	for scanner.Scan() {
		p.parseLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan turn records: %w", err)
	}

	return nil
}

func (p *turnParser) parseLine(line []byte) {
	var record rawRecord
	if json.Unmarshal(line, &record) != nil || !p.isWithinCutoff(record.Timestamp) {
		return
	}

	switch record.Type {
	case "event_msg":
		p.parseEvent(record.Payload, record.Timestamp)
	case "turn_context":
		p.parseContext(record.Payload)
	case "response_item":
		p.parseResponse(record.Payload)
	}
}

func (p *turnParser) isWithinCutoff(timestamp string) bool {
	if p.before.IsZero() || timestamp == "" {
		return true
	}

	recordTime, err := time.Parse(time.RFC3339Nano, timestamp)

	return err != nil || !recordTime.After(p.before)
}

func (p *turnParser) parseEvent(data json.RawMessage, timestamp string) {
	var payload eventPayload
	if json.Unmarshal(data, &payload) != nil {
		return
	}
	p.applyEvent(payload, timestamp)
}

func (p *turnParser) applyEvent(payload eventPayload, timestamp string) {
	switch payload.Type {
	case "task_started":
		p.start(payload.TurnID, timestamp)
	case "token_count":
		p.updateTokenTotal(payload)
	case "task_complete":
		p.complete(payload, true)
	case "turn_aborted":
		p.complete(payload, false)
	}
}

func (p *turnParser) start(id, timestamp string) {
	p.current = &Turn{
		ID: id, Status: "incomplete", Model: "unknown", Effort: "unknown", StartedAt: timestamp,
	}
	p.startTotal = p.lastTotal
}

func (p *turnParser) updateTokenTotal(payload eventPayload) {
	if payload.Info != nil {
		p.lastTotal = payload.Info.TotalTokenUsage.TotalTokens
	}
}

func (p *turnParser) complete(payload eventPayload, completed bool) {
	if p.current == nil {
		return
	}
	p.current.Tokens = p.lastTotal - p.startTotal
	p.current.DurationMS = payload.DurationMS
	if completed {
		p.current.Status = "complete"
		p.current.TTFTMS = payload.TTFTMS
	} else {
		p.current.Status = "aborted"
	}
	p.turns = append(p.turns, *p.current)
	p.current = nil
}

func (p *turnParser) parseContext(data json.RawMessage) {
	if p.current == nil {
		return
	}
	var payload turnContextPayload
	if json.Unmarshal(data, &payload) != nil || payload.TurnID != p.current.ID {
		return
	}
	if payload.Model != "" {
		p.current.Model = payload.Model
	}
	if payload.Effort != "" {
		p.current.Effort = payload.Effort
	}
}

func (p *turnParser) parseResponse(data json.RawMessage) {
	if p.current == nil {
		return
	}
	var payload responsePayload
	if json.Unmarshal(data, &payload) != nil || !isToolCall(payload.Type) {
		return
	}
	p.current.ToolCalls++
	if isSpawnAgentTool(payload.Name) || isSpawnAgentTool(payload.ToolName) {
		p.current.UsesSubagents = true
	}
}

func isToolCall(recordType string) bool {
	return recordType == "function_call" || recordType == "custom_tool_call"
}

func (p *turnParser) finish() []Turn {
	if p.current != nil {
		p.current.Tokens = p.lastTotal - p.startTotal
		p.turns = append(p.turns, *p.current)
	}

	return p.turns
}

func isSpawnAgentTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "spawn_agent" {
		return true
	}

	return strings.HasSuffix(name, ".spawn_agent") ||
		strings.HasSuffix(name, "/spawn_agent") ||
		strings.HasSuffix(name, ":spawn_agent") ||
		strings.HasSuffix(name, "__spawn_agent")
}
