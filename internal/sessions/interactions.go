package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/fileio"
)

const (
	scannerInitialCapacity = 64 * 1024
	interactionMaxLineSize = 16 * 1024 * 1024
)

type Interaction struct {
	TurnID    string
	StartedAt string
	Status    string
	Prompt    string
	Answer    string
}

type interactionRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type interactionPayload struct {
	Type             string                   `json:"type"`
	TurnID           string                   `json:"turn_id"`
	Message          string                   `json:"message"`
	LastAgentMessage string                   `json:"last_agent_message"`
	Item             interactionCompletedItem `json:"item"`
}

type interactionCompletedItem struct {
	Type    string                   `json:"type"`
	Content []interactionContentItem `json:"content"`
}

type interactionResponseItem struct {
	Type     string                   `json:"type"`
	Role     string                   `json:"role"`
	Content  []interactionContentItem `json:"content"`
	Metadata struct {
		TurnID           string   `json:"turn_id"`
		ContentItemKinds []string `json:"content_item_kinds"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

type interactionContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type interactionParser struct {
	before  time.Time
	result  []Interaction
	current *Interaction
}

func ParseInteractions(path string, before time.Time) ([]Interaction, error) {
	parser := interactionParser{before: before}
	if err := fileio.Read(path, parser.scan); err != nil {
		return nil, fmt.Errorf("parse interactions from %q: %w", path, err)
	}

	return parser.finish(), nil
}

func (p *interactionParser) scan(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, scannerInitialCapacity), interactionMaxLineSize)
	for scanner.Scan() {
		p.parseLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan interaction records: %w", err)
	}

	return nil
}

func (p *interactionParser) parseLine(line []byte) {
	var record interactionRecord
	if json.Unmarshal(line, &record) != nil || !p.isWithinCutoff(record.Timestamp) {
		return
	}

	switch record.Type {
	case "response_item":
		p.parseResponse(record.Payload)
	case "event_msg":
		p.parseEvent(record.Timestamp, record.Payload)
	}
}

func (p *interactionParser) isWithinCutoff(timestamp string) bool {
	if p.before.IsZero() || timestamp == "" {
		return true
	}

	recordTime, err := time.Parse(time.RFC3339Nano, timestamp)

	return err != nil || !recordTime.After(p.before)
}

func (p *interactionParser) parseResponse(payload json.RawMessage) {
	var item interactionResponseItem
	if json.Unmarshal(payload, &item) != nil || p.current == nil || item.Type != "message" || item.Role != "user" {
		return
	}
	if item.Metadata.TurnID != p.current.TurnID || p.current.Prompt != "" ||
		!containsInteractionKind(item.Metadata.ContentItemKinds, "user.text") {
		return
	}

	p.current.Prompt = interactionInputText(item.Content)
}

func (p *interactionParser) parseEvent(timestamp string, data json.RawMessage) {
	var payload interactionPayload
	if json.Unmarshal(data, &payload) != nil {
		return
	}

	switch payload.Type {
	case "task_started":
		p.start(payload.TurnID, timestamp)
	case "user_message":
		p.setLegacyPrompt(payload.Message)
	case "item_completed":
		p.setCompletedPrompt(payload)
	case "task_complete":
		p.complete(payload)
	case "turn_aborted":
		p.abort()
	}
}

func (p *interactionParser) start(turnID, timestamp string) {
	if p.current != nil {
		if p.current.TurnID == turnID && turnID != "" {
			return
		}
		p.result = append(p.result, *p.current)
	}
	p.current = &Interaction{TurnID: turnID, StartedAt: timestamp, Status: "incomplete"}
}

func (p *interactionParser) setLegacyPrompt(prompt string) {
	if p.current != nil && p.current.Prompt == "" {
		p.current.Prompt = prompt
	}
}

func (p *interactionParser) setCompletedPrompt(payload interactionPayload) {
	if p.current == nil || payload.TurnID != p.current.TurnID || payload.Item.Type != "UserMessage" {
		return
	}
	if prompt := interactionText(payload.Item.Content); prompt != "" {
		// Completed user text is authoritative over injected rollout context.
		p.current.Prompt = prompt
	}
}

func (p *interactionParser) complete(payload interactionPayload) {
	if p.current == nil {
		return
	}
	p.current.Status = "complete"
	p.current.Answer = payload.LastAgentMessage
	p.result = append(p.result, *p.current)
	p.current = nil
}

func (p *interactionParser) abort() {
	if p.current == nil {
		return
	}
	p.current.Status = "aborted"
	p.result = append(p.result, *p.current)
	p.current = nil
}

func (p *interactionParser) finish() []Interaction {
	if p.current != nil {
		p.result = append(p.result, *p.current)
	}

	return p.result
}

func containsInteractionKind(kinds []string, expected string) bool {
	return slices.Contains(kinds, expected)
}

func interactionInputText(content []interactionContentItem) string {
	var prompt strings.Builder
	for _, item := range content {
		if item.Type == "input_text" && item.Text != "" {
			prompt.WriteString(item.Text)
		}
	}

	return prompt.String()
}

func interactionText(content []interactionContentItem) string {
	for _, item := range content {
		if item.Type == "text" && item.Text != "" {
			return item.Text
		}
	}

	return ""
}
