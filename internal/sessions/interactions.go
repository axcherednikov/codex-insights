package sessions

import (
	"bufio"
	"encoding/json"
	"os"
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
	Type             string `json:"type"`
	TurnID           string `json:"turn_id"`
	Message          string `json:"message"`
	LastAgentMessage string `json:"last_agent_message"`
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
