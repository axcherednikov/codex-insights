package sessions

import (
	"bufio"
	"encoding/json"
	"os"
)

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
	f, err := os.Open(path)
	if err != nil {
		return SessionMeta{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		if event.Type != "session_meta" {
			continue
		}

		var meta SessionMeta
		if err := json.Unmarshal(event.Payload, &meta); err != nil {
			return SessionMeta{}, err
		}
		meta.StartedAt = event.Timestamp

		return meta, nil
	}

	return SessionMeta{}, scanner.Err()
}
