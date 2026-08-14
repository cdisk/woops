package opsctl

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gorilla/websocket"
)

type execClientMsg struct {
	Type       string `json:"type"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

type execServerMsg struct {
	Type       string `json:"type"`
	Stream     string `json:"stream,omitempty"`
	Data       string `json:"data,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ExecResult is the remote process outcome.
type ExecResult struct {
	ExitCode   int
	DurationMs int64
}

func Exec(cfg Config, wsURL, command, cwd string, timeoutSec int, stdout, stderr io.Writer) (*ExecResult, error) {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	dialer := cfg.dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	defer ws.Close()

	req := execClientMsg{
		Type:       "run",
		Command:    command,
		Cwd:        cwd,
		TimeoutSec: timeoutSec,
	}
	if err := ws.WriteJSON(req); err != nil {
		return nil, err
	}

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("ws read: %w", err)
		}
		if len(raw) > 0 && raw[0] != '{' {
			return nil, fmt.Errorf("%s", string(raw))
		}
		var msg execServerMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("bad exec frame: %w", err)
		}
		switch msg.Type {
		case "output":
			w := stdout
			if msg.Stream == "stderr" {
				w = stderr
			}
			if _, err := io.WriteString(w, msg.Data); err != nil {
				return nil, err
			}
		case "done":
			code := 0
			if msg.ExitCode != nil {
				code = *msg.ExitCode
			}
			return &ExecResult{ExitCode: code, DurationMs: msg.DurationMs}, nil
		case "error":
			return nil, fmt.Errorf("%s", msg.Error)
		default:
			// ignore unknown
		}
	}
}
