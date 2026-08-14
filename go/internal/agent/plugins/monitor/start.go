package monitor

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

type Config struct {
	MetricsWS  string
	AssetID    string
	AgentToken string
	Interval   time.Duration
	Enabled    bool
	// ConfigPath is agent.yaml; when set, credentials are re-read before each dial.
	ConfigPath string
	// Dialer applies the same TLS/SPKI pin policy as control/session; nil → DefaultDialer.
	Dialer *websocket.Dialer
}

// Start dials the dedicated metrics WSS and reports points on Interval.
// It reconnects until ctx is cancelled. Does not affect agent online status.
func Start(ctx context.Context, cfg Config) {
	if !cfg.Enabled {
		log.Printf("metrics disabled")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	for {
		if err := runOnce(ctx, cfg); err != nil && ctx.Err() == nil {
			log.Printf("metrics ended err=%v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func runOnce(ctx context.Context, cfg Config) error {
	if cfg.ConfigPath != "" {
		dir := filepath.Dir(cfg.ConfigPath)
		if b, err := os.ReadFile(filepath.Join(dir, "asset-id")); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				cfg.AssetID = id
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "agent-token")); err == nil {
			if tok := strings.TrimSpace(string(b)); tok != "" {
				cfg.AgentToken = tok
			}
		}
	}
	header := http.Header{}
	header.Set("X-Asset-Id", cfg.AssetID)
	header.Set("X-Agent-Token", cfg.AgentToken)
	d := cfg.Dialer
	if d == nil {
		d = websocket.DefaultDialer
	}
	ws, _, err := d.DialContext(ctx, cfg.MetricsWS, header)
	if err != nil {
		return err
	}
	defer ws.Close()
	log.Printf("metrics connected")

	// Unblock ReadMessage when agent is stopping (read deadline alone can wait ~120s).
	go func() {
		<-ctx.Done()
		_ = ws.Close()
	}()

	wsutil.EnableReadDeadline(ws, wsutil.MetricsIdle)

	var writeMu sync.Mutex
	send := func() error {
		points := Collect()
		msg, err := monitorproto.Marshal("points", "", monitorproto.PointsPayload{
			CollectedAt: time.Now().UTC().Format(time.RFC3339),
			Points:      points,
		})
		if err != nil {
			return err
		}
		writeMu.Lock()
		err = ws.WriteMessage(websocket.TextMessage, msg)
		writeMu.Unlock()
		if err != nil {
			return err
		}
		// Success is silent: per-interval "sent N points" flooded journald.
		return nil
	}

	// First sample after a short delay so rate diffs have a baseline on second tick.
	_ = send()

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)

	wsutil.StartPingLoop(ws, done, wsutil.MetricsIdle, wsutil.DefaultPing, &writeMu)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := send(); err != nil {
					_ = ws.Close()
					return
				}
			}
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		_ = ws.SetReadDeadline(time.Now().Add(wsutil.MetricsIdle))
		var env monitorproto.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		// ack / pong ignored
		_ = env
	}
}
