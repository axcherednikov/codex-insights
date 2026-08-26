package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

const JudgeMarker = "[CODEX_INSIGHTS_JUDGE_V1]"

func IsInsightsJudgeSession(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		if !bytes.Contains(line, []byte(JudgeMarker)) {
			continue
		}

		var record struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}

		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		switch record.Type {
		case "event_msg":
			var payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}

			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}

			if payload.Type == "user_message" &&
				strings.Contains(payload.Message, JudgeMarker) {
				return true, nil
			}

		case "response_item":
			var payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}

			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}

			if payload.Type != "message" || payload.Role != "user" {
				continue
			}

			for _, content := range payload.Content {
				if strings.Contains(content.Text, JudgeMarker) {
					return true, nil
				}
			}
		}
	}

	return false, scanner.Err()
}
