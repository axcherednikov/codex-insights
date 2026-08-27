package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

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

	Info *struct {
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

func ParseTurns(path string, before time.Time) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		turns      []Turn
		current    *Turn
		startTotal int64
		lastTotal  int64
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		var record rawRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}

		if !before.IsZero() && record.Timestamp != "" {
			recordTime, err := time.Parse(time.RFC3339Nano, record.Timestamp)
			if err == nil && recordTime.After(before) {
				continue
			}
		}

		switch record.Type {
		case "event_msg":
			var payload eventPayload
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}

			switch payload.Type {
			case "task_started":
				current = &Turn{
					ID:        payload.TurnID,
					Status:    "incomplete",
					Model:     "unknown",
					Effort:    "unknown",
					StartedAt: record.Timestamp,
				}
				startTotal = lastTotal

			case "token_count":
				if payload.Info != nil {
					lastTotal = payload.Info.TotalTokenUsage.TotalTokens
				}

			case "task_complete":
				if current != nil {
					current.Status = "complete"
					current.Tokens = lastTotal - startTotal
					current.DurationMS = payload.DurationMS
					current.TTFTMS = payload.TTFTMS
					turns = append(turns, *current)
					current = nil
				}

			case "turn_aborted":
				if current != nil {
					current.Status = "aborted"
					current.Tokens = lastTotal - startTotal
					current.DurationMS = payload.DurationMS
					turns = append(turns, *current)
					current = nil
				}
			}

		case "turn_context":
			if current == nil {
				continue
			}

			var payload turnContextPayload
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}

			if payload.TurnID == current.ID {
				if payload.Model != "" {
					current.Model = payload.Model
				}

				if payload.Effort != "" {
					current.Effort = payload.Effort
				}
			}

		case "response_item":
			if current == nil {
				continue
			}

			var payload responsePayload
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}

			if payload.Type == "function_call" ||
				payload.Type == "custom_tool_call" {
				current.ToolCalls++
				if isSpawnAgentTool(payload.Name) || isSpawnAgentTool(payload.ToolName) {
					current.UsesSubagents = true
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if current != nil {
		current.Tokens = lastTotal - startTotal
		turns = append(turns, *current)
	}

	return turns, nil
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
