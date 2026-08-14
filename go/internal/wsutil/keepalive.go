// Package wsutil holds shared WebSocket keepalive helpers for Gateway and Agent.
package wsutil

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultIdle = 90 * time.Second
	DefaultPing = 25 * time.Second
	MetricsIdle = 120 * time.Second
)

// EnableReadDeadline refreshes the read deadline on inbound Ping/Pong.
// Use on connections that always have a reader (control, shell, metrics).
// Do not use on request/response file RPC: a long write with no Read would
// expire the deadline and kill the next read.
func EnableReadDeadline(ws *websocket.Conn, idle time.Duration) {
	if ws == nil {
		return
	}
	if idle <= 0 {
		idle = DefaultIdle
	}
	refresh := func() {
		_ = ws.SetReadDeadline(time.Now().Add(idle))
	}
	refresh()
	ws.SetPingHandler(func(appData string) error {
		err := ws.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
		refresh()
		return err
	})
	ws.SetPongHandler(func(string) error {
		refresh()
		return nil
	})
}

// StartPingLoop sends protocol Ping frames until stop is closed.
// If writeMu is non-nil, each WriteControl is serialized with it.
func StartPingLoop(ws *websocket.Conn, stop <-chan struct{}, idle, pingEvery time.Duration, writeMu *sync.Mutex) {
	if ws == nil {
		return
	}
	if idle <= 0 {
		idle = DefaultIdle
	}
	if pingEvery <= 0 {
		pingEvery = DefaultPing
	}
	EnableReadDeadline(ws, idle)
	go func() {
		ticker := time.NewTicker(pingEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				deadline := time.Now().Add(5 * time.Second)
				var err error
				if writeMu != nil {
					writeMu.Lock()
					err = ws.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
					writeMu.Unlock()
				} else {
					err = ws.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
				}
				if err != nil {
					return
				}
			}
		}
	}()
}
