// Package session records a human-readable, append-only transcript of a run to
// disk. This gives D&R work an audit trail: what the operator asked, what the
// agent said, and every tool call it made (with results).
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// EntryKind classifies a transcript entry.
type EntryKind string

const (
	KindUser       EntryKind = "user"
	KindAssistant  EntryKind = "assistant"
	KindToolCall   EntryKind = "tool_call"
	KindToolResult EntryKind = "tool_result"
)

// Entry is one line in the transcript.
type Entry struct {
	Time    time.Time `json:"time"`
	Kind    EntryKind `json:"kind"`
	Tool    string    `json:"tool,omitempty"`
	Content string    `json:"content"`
	IsError bool      `json:"is_error,omitempty"`
}

// Session accumulates entries and flushes them to a JSON file.
type Session struct {
	ID      string  `json:"id"`
	Started string  `json:"started"`
	Entries []Entry `json:"entries"`

	path string
}

// New creates a session whose transcript will be written under dir (typically
// the user data dir). A nil error with an empty path means persistence is
// disabled; the session still works in memory.
func New(dir string) (*Session, error) {
	id := time.Now().Format("20060102-150405")
	s := &Session{ID: id, Started: time.Now().Format(time.RFC3339)}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return s, err
	}
	s.path = filepath.Join(dir, id+".json")
	return s, nil
}

// Path returns the transcript file path, or "" if persistence is disabled.
func (s *Session) Path() string { return s.path }

// Add appends an entry and flushes the transcript to disk.
func (s *Session) Add(e Entry) {
	e.Time = time.Now()
	s.Entries = append(s.Entries, e)
	s.flush()
}

func (s *Session) flush() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	// Best-effort: a failed flush should never abort the run.
	_ = os.WriteFile(s.path, data, 0o644)
}

// DefaultDir returns the standard transcript directory for the host.
func DefaultDir() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(dir, ".local", "share", "vala", "sessions")
	}
	return ""
}
