package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type LogEntry struct {
	Contact string    `json:"contact"`
	Time    time.Time `json:"time"`
	Note    string    `json:"note,omitempty"`
}

func logFilePath() string {
	return filepath.Join(configDir(), "log.jsonl")
}

func appendLog(entry LogEntry) error {
	path := logFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling log entry: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing log entry: %w", err)
	}
	return nil
}

func readLog() ([]LogEntry, error) {
	path := logFilePath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// lastContactTime returns the most recent log time for each contact name.
func lastContactTime(entries []LogEntry) map[string]time.Time {
	last := make(map[string]time.Time)
	for _, e := range entries {
		if e.Time.After(last[e.Contact]) {
			last[e.Contact] = e.Time
		}
	}
	return last
}
