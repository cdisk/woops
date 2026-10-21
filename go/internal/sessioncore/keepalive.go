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
	// AgentDialWait is how long Gateway waits for the Agent to dial the data socket.
	AgentDialWait = 20 * time.Second
)

// StartKeepAlive sends protocol Ping frames at the default 25s interval and
// fails the read side if the peer stops answering within KeepAliveIdle.
// woopsctl uses this; Gateway session bridges use StartKeepAliveEvery.
func StartKeepAlive(ws *websocket.Conn, stop <-chan struct{}) {
	StartKeepAliveEvery(ws, stop, KeepAlivePing)
}

// StartKeepAliveEvery pings at pingEvery. The session read timeout stays
// KeepAliveIdle (90s) so a slower configured ping still has slack.
func StartKeepAliveEvery(ws *websocket.Conn, stop <-chan struct{}, pingEvery time.Duration) {
	if pingEvery <= 0 {
		pingEvery = KeepAlivePing
	}
	wsutil.StartPingLoop(ws, stop, KeepAliveIdle, pingEvery, nil)
}
