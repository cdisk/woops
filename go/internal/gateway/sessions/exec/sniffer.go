package exec

import (
	"encoding/json"
	"strings"
	"sync"
)

// Sniffer observes the opsctl/exec JSON protocol and emits a single RUN
// ACTION (command metadata) plus captures exit/error for the OPERATION END.
// Stdout/stderr bodies are never recorded.
type Sniffer struct {
	emit func(eventType string, detail map[string]any)

	mu         sync.Mutex
	command    string
	cwd        string
	timeoutSec int
	exitCode   *int
	durationMs int64
	errMsg     string
	ran        bool
}

type execFrame struct {
	Type       string `json:"type"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	TimeoutSec int    `json:"timeoutSec"`
	ExitCode   *int   `json:"exitCode"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error"`
}

// NewSniffer builds an exec protocol sniffer.
func NewSniffer(emit func(eventType string, detail map[string]any)) *Sniffer {
	return &Sniffer{emit: emit}
}

// ObserveClient inspects browser/opsctl → agent frames.
func (s *Sniffer) ObserveClient(frame []byte) {
	if s == nil || s.emit == nil {
		return
	}
	var msg execFrame
	if err := json.Unmarshal(frame, &msg); err != nil {
		return
	}
	if msg.Type != "run" || msg.Command == "" {
		return
	}
	s.mu.Lock()
	if s.ran {
		s.mu.Unlock()
		return
	}
	s.ran = true
	s.command = truncateStr(msg.Command, 2000)
	s.cwd = msg.Cwd
	s.timeoutSec = msg.TimeoutSec
	detail := map[string]any{"command": s.command}
	if s.cwd != "" {
		detail["cwd"] = s.cwd
	}
	if s.timeoutSec > 0 {
		detail["timeoutSec"] = s.timeoutSec
	}
	s.mu.Unlock()
	s.emit("RUN", detail)
}

// ObserveServer inspects agent → client frames for done/error.
func (s *Sniffer) ObserveServer(frame []byte) {
	if s == nil {
		return
	}
	var msg execFrame
	if err := json.Unmarshal(frame, &msg); err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch msg.Type {
	case "done":
		s.exitCode = msg.ExitCode
		s.durationMs = msg.DurationMs
	case "error":
		s.errMsg = truncateStr(msg.Error, 500)
	}
}

// EndDetail returns fields to merge into the OPERATION END envelope.
func (s *Sniffer) EndDetail() map[string]any {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	if s.command != "" {
		out["command"] = s.command
	}
	if s.cwd != "" {
		out["cwd"] = s.cwd
	}
	if s.timeoutSec > 0 {
		out["timeoutSec"] = s.timeoutSec
	}
	if s.exitCode != nil {
		out["exitCode"] = *s.exitCode
	}
	if s.durationMs > 0 {
		out["durationMs"] = s.durationMs
	}
	if s.errMsg != "" {
		out["error"] = s.errMsg
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Success reports whether the remote process finished with exit 0.
// Unknown (no done frame) returns true so a transport drop is not double-failed
// beyond the bridge's own failure path.
func (s *Sniffer) Success() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errMsg != "" {
		return false
	}
	if s.exitCode != nil {
		return *s.exitCode == 0
	}
	return true
}

func truncateStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
