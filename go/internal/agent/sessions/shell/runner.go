package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

const sessionKeepAliveIdle = 90 * time.Second

// How long to wait for the browser's first R,cols,rows before spawning the shell.
// ConPTY/PSReadLine often keep the initial geometry for absolute CUP redraws; a
// late Resize after the prompt is painted is unreliable and causes viewport overwrite.
// Linux creack/pty accepts late Setsize fine — skip the wait there.
const initialSizeWait = 2 * time.Second

const defaultCols = 120
const defaultRows = 40

// Run parses shell open-session parameters and owns the complete shell session.
func Run(ws *websocket.Conn, params json.RawMessage) error {
	var p control.ShellParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return err
		}
	}
	cols, rows := normalizeSize(p.Cols, p.Rows)
	if runtime.GOOS == "windows" {
		cols, rows = awaitInitialSize(ws, cols, rows)
	}

	sess, err := Start(p.ShellKind, cols, rows)
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("ERROR start shell (%s): %v", p.ShellKind, err)))
		time.Sleep(200 * time.Millisecond)
		return err
	}
	defer sess.Close()

	wsutil.EnableReadDeadline(ws, sessionKeepAliveIdle)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(sessionKeepAliveIdle))
			if mt == websocket.TextMessage {
				var c, r int
				if _, err := fmt.Sscanf(string(data), "R,%d,%d", &c, &r); err == nil && c > 0 && r > 0 {
					c, r = normalizeSize(c, r)
					if err := sess.Resize(c, r); err != nil {
						log.Printf("shell: resize %dx%d failed: %v", c, r, err)
					}
				}
				continue
			}
			if _, err := sess.Write(data); err != nil {
				return
			}
		}
	}()

	go func() {
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	<-ctx.Done()
	return nil
}

func normalizeSize(cols, rows int) (int, int) {
	if cols <= 0 {
		cols = defaultCols
	}
	if rows <= 0 {
		rows = defaultRows
	}
	if cols > 1000 {
		cols = 1000
	}
	if rows > 500 {
		rows = 500
	}
	return cols, rows
}

// awaitInitialSize blocks briefly for the browser's first geometry frame so the
// PTY can be created at the real size. Falls back to the provided defaults.
func awaitInitialSize(ws *websocket.Conn, fallbackCols, fallbackRows int) (cols, rows int) {
	cols, rows = fallbackCols, fallbackRows
	_ = ws.SetReadDeadline(time.Now().Add(initialSizeWait))
	defer func() { _ = ws.SetReadDeadline(time.Time{}) }()
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			log.Printf("shell: initial size wait ended (%dx%d): %v", cols, rows, err)
			return cols, rows
		}
		if mt != websocket.TextMessage {
			continue
		}
		var c, r int
		if _, err := fmt.Sscanf(string(data), "R,%d,%d", &c, &r); err == nil && c > 0 && r > 0 {
			c, r = normalizeSize(c, r)
			log.Printf("shell: initial size from browser %dx%d", c, r)
			return c, r
		}
	}
}
