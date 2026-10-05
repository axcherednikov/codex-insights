package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/axcherednikov/codex-insights/internal/fileio"
)

const metaMaxLineSize = 10 * 1024 * 1024

type Event struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type SessionMeta struct {
	ID           string `json:"id"`
	Originator   string `json:"originator"`
	ThreadSource string `json:"thread_source"`
	Source       any    `json:"source"`
	StartedAt    string `json:"-"`
}

func ReadMeta(path string) (SessionMeta, error) {
	var meta SessionMeta
	err := fileio.Read(path, func(reader io.Reader) error {
		parsed, parseErr := parseMeta(reader)
		meta = parsed

		return parseErr
	})
	if err != nil {
		return SessionMeta{}, fmt.Errorf("read session metadata from %q: %w", path, err)
	}

	return meta, nil
}

func parseMeta(reader io.Reader) (SessionMeta, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, scannerInitialCapacity), metaMaxLineSize)
	for scanner.Scan() {
		meta, found, err := parseMetaLine(scanner.Bytes())
		if err != nil {
			return SessionMeta{}, err
		}
		if found {
			return meta, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return SessionMeta{}, fmt.Errorf("scan session metadata records: %w", err)
	}

	return SessionMeta{}, nil
}

func parseMetaLine(line []byte) (SessionMeta, bool, error) {
	var event Event
	if json.Unmarshal(line, &event) != nil || event.Type != "session_meta" {
		return SessionMeta{}, false, nil
	}

	var meta SessionMeta
	if err := json.Unmarshal(event.Payload, &meta); err != nil {
		return SessionMeta{}, false, fmt.Errorf("decode session metadata payload: %w", err)
	}
	meta.StartedAt = event.Timestamp

	return meta, true, nil
}
