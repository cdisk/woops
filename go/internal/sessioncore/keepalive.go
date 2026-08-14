package sessioncore

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

// Session data WSS (shell/file/exec/desktop) previously had no keepalive, so a
// half-open peer left ReadMessage blocked and audit stuck in RUNNING.
const (
	KeepAliveIdle = 90 * time.Second
	KeepAlivePing = 25 * time.Second
)

// StartKeepAlive sends protocol Ping frames and fails the read side if the
// peer stops answering (or sending data) within KeepAliveIdle. Control frames
// are handled by gorilla outside the Text/Binary pipe, so application protocols
// need no new signaling. stop ends the ping loop (typically when the bridge exits).
func StartKeepAlive(ws *websocket.Conn, stop <-chan struct{}) {
	wsutil.StartPingLoop(ws, stop, KeepAliveIdle, KeepAlivePing, nil)
}
