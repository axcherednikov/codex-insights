package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/axcherednikov/codex-insights/internal/fileio"
)

const JudgeMarker = "[CODEX_INSIGHTS_JUDGE_V1]"

type judgeSessionRecord struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type judgeSessionParser struct {
	found bool
}

func IsInsightsJudgeSession(path string) (bool, error) {
	parser := judgeSessionParser{}
	err := fileio.Read(path, parser.scan)
	if err != nil {
		return false, fmt.Errorf("inspect Judge session %q: %w", path, err)
	}

	return parser.found, nil
}

func (p *judgeSessionParser) scan(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, scannerInitialCapacity), turnMaxLineSize)
	for scanner.Scan() && !p.found {
		p.parseLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan Judge session records: %w", err)
	}

	return nil
}

func (p *judgeSessionParser) parseLine(line []byte) {
	if !bytes.Contains(line, []byte(JudgeMarker)) {
		return
	}
	var record judgeSessionRecord
	if json.Unmarshal(line, &record) != nil {
		return
	}

	switch record.Type {
	case "event_msg":
		p.parseEvent(record.Payload)
	case "response_item":
		p.parseResponse(record.Payload)
	}
}

func (p *judgeSessionParser) parseEvent(data json.RawMessage) {
	var payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &payload) == nil && payload.Type == "user_message" {
		p.found = strings.Contains(payload.Message, JudgeMarker)
	}
}

func (p *judgeSessionParser) parseResponse(data json.RawMessage) {
	var payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.Type != "message" || payload.Role != "user" {
		return
	}
	for _, content := range payload.Content {
		if strings.Contains(content.Text, JudgeMarker) {
			p.found = true

			return
		}
	}
}
