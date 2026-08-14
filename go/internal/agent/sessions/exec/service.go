package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type clientMsg struct {
	Type       string `json:"type"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

type serverMsg struct {
	Type       string `json:"type"`
	Stream     string `json:"stream,omitempty"` // stdout | stderr
	Data       string `json:"data,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

const (
	defaultTimeoutSec = 300
	maxTimeoutSec     = 3600
	maxOutputChunk    = 32 * 1024
)

// ServeJSON runs one non-interactive command over the session WebSocket.
// Protocol: client sends {"type":"run","command":"...","cwd":"...","timeoutSec":300};
// server streams {"type":"output","stream":"stdout|stderr","data":"..."} then
// {"type":"done","exitCode":N,"durationMs":M} (or {"type":"error","error":"..."}).
func ServeJSON(ws *websocket.Conn) {
	_, data, err := ws.ReadMessage()
	if err != nil {
		return
	}
	var msg clientMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: "bad json"})
		return
	}
	if msg.Type != "run" {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: "expected type=run"})
		return
	}
	if msg.Command == "" {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: "command required"})
		return
	}
	timeout := msg.TimeoutSec
	if timeout <= 0 {
		timeout = defaultTimeoutSec
	}
	if timeout > maxTimeoutSec {
		timeout = maxTimeoutSec
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Cancel on client disconnect.
	go func() {
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	cmd, err := buildCmd(ctx, msg.Command, msg.Cwd)
	if err != nil {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: err.Error()})
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: err.Error()})
		return
	}

	started := time.Now()
	if err := cmd.Start(); err != nil {
		_ = writeMsg(ws, serverMsg{Type: "error", Error: err.Error()})
		return
	}

	var writeMu sync.Mutex
	var wg sync.WaitGroup
	pump := func(r io.Reader, stream string) {
		defer wg.Done()
		buf := make([]byte, maxOutputChunk)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = writeMsg(ws, serverMsg{Type: "output", Stream: stream, Data: string(buf[:n])})
				writeMu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(stdout, "stdout")
	go pump(stderr, "stderr")
	wg.Wait()

	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else if ctx.Err() != nil {
			exitCode = 124 // conventional timeout
		} else {
			writeMu.Lock()
			_ = writeMsg(ws, serverMsg{Type: "error", Error: waitErr.Error()})
			writeMu.Unlock()
			return
		}
	}
	if ctx.Err() == context.DeadlineExceeded && exitCode == 0 {
		exitCode = 124
	}

	dur := time.Since(started).Milliseconds()
	writeMu.Lock()
	_ = writeMsg(ws, serverMsg{Type: "done", ExitCode: &exitCode, DurationMs: dur})
	writeMu.Unlock()
}

func buildCmd(ctx context.Context, command, cwd string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-lc", command)
	}
	if cwd != "" {
		if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("invalid cwd: %s", cwd)
		}
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	return cmd, nil
}

func writeMsg(ws *websocket.Conn, m serverMsg) error {
	return ws.WriteJSON(m)
}
