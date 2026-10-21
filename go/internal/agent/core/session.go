package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

const (
	sessionDialBudget         = 18 * time.Second
	sessionDialRetryMinRemain = 5 * time.Second
)

func (r *Runtime) handleOpenSession(raw control.OpenSessionPayload) {
	p := control.NormalizeOpenSession(raw)
	opType := r.deps.Sessions.OperationType(p.Protocol)
	log.Printf("op START type=%s session=%s", opType, p.SessionID)

	ws, err := r.dialSessionWS(p.SessionID, p.Ticket)
	if err != nil {
		log.Printf("op FAIL type=%s session=%s err=%q", opType, p.SessionID, err.Error())
		return
	}
	defer ws.Close()

	failed := false
	if err := r.dispatchSession(ws, p); err != nil {
		failed = true
		log.Printf("op FAIL type=%s session=%s err=%q", opType, p.SessionID, err.Error())
	}
	if !failed {
		log.Printf("op END type=%s session=%s", opType, p.SessionID)
	}
}

func (r *Runtime) dispatchSession(ws *websocket.Conn, p control.OpenSessionPayload) error {
	return r.deps.Sessions.Run(p.Protocol, ws, p.Params)
}

func (r *Runtime) dialSessionWS(sessionID, ticket string) (*websocket.Conn, error) {
	if err := r.cfg.ReloadCredentials(); err != nil {
		log.Printf("reload credentials: %v", err)
	}
	u, err := url.Parse(r.cfg.SessionWS)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("sessionId", sessionID)
	q.Set("ticket", ticket)
	u.RawQuery = q.Encode()
	header := http.Header{}
	header.Set("X-Asset-Id", r.cfg.AssetID)
	header.Set("X-Agent-Token", r.cfg.AgentToken)
	ctx, cancel := context.WithTimeout(context.Background(), sessionDialBudget)
	defer cancel()
	return dialWithRetry(ctx, func(ctx context.Context) (*websocket.Conn, error) {
		ws, _, err := r.dialer.DialContext(ctx, u.String(), header)
		return ws, err
	})
}

// dialWithRetry attempts dial once, then once more when the failure was fast
// and more than sessionDialRetryMinRemain is left in ctx.
func dialWithRetry(ctx context.Context, dial func(context.Context) (*websocket.Conn, error)) (*websocket.Conn, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) <= sessionDialRetryMinRemain {
				break
			}
		}
		ws, err := dial(ctx)
		if err == nil {
			return ws, nil
		}
		last = err
		if ctx.Err() != nil {
			break
		}
	}
	if last == nil {
		last = ctx.Err()
	}
	return nil, last
}

func (r *Runtime) postJSON(path string, body, out any) error {
	if r.http == nil {
		return fmt.Errorf("http client not ready")
	}
	endpoint, err := gatewayHTTPBase(r.cfg.Gateway)
	if err != nil {
		return err
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + strings.TrimLeft(path, "/")

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := r.cfg.ReloadCredentials(); err != nil {
		log.Printf("reload credentials: %v", err)
	}
	req.Header.Set("X-Asset-Id", r.cfg.AssetID)
	req.Header.Set("X-Agent-Token", r.cfg.AgentToken)
	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("agent request %s: %s", resp.Status, string(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
