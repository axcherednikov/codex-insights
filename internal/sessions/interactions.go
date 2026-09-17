package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"time"
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

func ParseInteractions(path string, before time.Time) ([]Interaction, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		result  []Interaction
		current *Interaction
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		var record interactionRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}

		if !before.IsZero() && record.Timestamp != "" {
			recordTime, err := time.Parse(time.RFC3339Nano, record.Timestamp)
			if err == nil && recordTime.After(before) {
				continue
			}
		}

		if record.Type == "response_item" {
			var item interactionResponseItem
			if err := json.Unmarshal(record.Payload, &item); err != nil {
				continue
			}
			if current != nil &&
				item.Type == "message" &&
				item.Role == "user" &&
				item.Metadata.TurnID == current.TurnID &&
				containsInteractionKind(item.Metadata.ContentItemKinds, "user.text") &&
				current.Prompt == "" {
				current.Prompt = interactionInputText(item.Content)
			}
			continue
		}

		if record.Type != "event_msg" {
			continue
		}

		var payload interactionPayload
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			continue
		}

		switch payload.Type {
		case "task_started":
			current = &Interaction{
				TurnID:    payload.TurnID,
				StartedAt: record.Timestamp,
				Status:    "incomplete",
			}

		case "user_message":
			if current != nil && current.Prompt == "" {
				current.Prompt = payload.Message
			}

		case "item_completed":
			if current != nil && payload.TurnID == current.TurnID && payload.Item.Type == "UserMessage" {
				if prompt := interactionText(payload.Item.Content); prompt != "" {
					// This event identifies the actual user message for the turn and
					// is authoritative when a rollout also contains injected context.
					current.Prompt = prompt
				}
			}

		case "task_complete":
			if current != nil {
				current.Status = "complete"
				current.Answer = payload.LastAgentMessage
				result = append(result, *current)
				current = nil
			}

		case "turn_aborted":
			if current != nil {
				current.Status = "aborted"
				result = append(result, *current)
				current = nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if current != nil {
		result = append(result, *current)
	}

	return result, nil
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
