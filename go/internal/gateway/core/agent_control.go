package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

func (s *Server) handleAgentControl(w http.ResponseWriter, r *http.Request) {
	assetID := r.Header.Get("X-Asset-Id")
	token := r.Header.Get("X-Agent-Token")
	if assetID == "" || token == "" {
		http.Error(w, "missing agent credentials", http.StatusUnauthorized)
		return
	}
	var auth map[string]any
	if err := s.postJSON("/api/internal/agent/auth", map[string]any{
		"assetId": assetID, "agentToken": token,
	}, &auth); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	peerIP := clientIP(r)
	connID := newConnectionID()
	conn := &AgentConn{AssetID: assetID, WS: ws, PeerIP: peerIP, ConnID: connID}
	s.agents.Put(conn)
	_ = s.postJSON("/api/internal/agent/online", map[string]any{
		"assetId":         assetID,
		"agentToken":      token,
		"online":          true,
		"sourceIp":        peerIP,
		"reason":          "connected",
		"connectionId":    connID,
		"gatewayInstance": s.instanceID,
	}, nil)
	log.Printf("agent online: %s (%v) peer=%s conn=%s", assetID, auth["displayName"], peerIP, connID)

	offlineReason := "disconnected"
	defer func() {
		wasCurrent := s.agents.Remove(assetID, conn)
		s.extensionsMu.RLock()
		closeHooks := append([]AgentLifecycleHook(nil), s.controlCloseHooks...)
		offlineHooks := append([]AgentLifecycleHook(nil), s.agentOfflineHooks...)
		s.extensionsMu.RUnlock()
		for _, hook := range closeHooks {
			hook(assetID)
		}
		if wasCurrent {
			for _, hook := range offlineHooks {
				hook(assetID)
			}
			_ = s.postJSON("/api/internal/agent/online", map[string]any{
				"assetId":         assetID,
				"agentToken":      token,
				"online":          false,
				"sourceIp":        peerIP,
				"reason":          offlineReason,
				"connectionId":    connID,
				"gatewayInstance": s.instanceID,
			}, nil)
			log.Printf("agent offline: %s reason=%s peer=%s conn=%s", assetID, offlineReason, peerIP, connID)
		} else {
			log.Printf("agent control superseded (skip offline): %s conn=%s", assetID, connID)
		}
		_ = ws.Close()
	}()

	hello, _ := control.Marshal("hello_ack", "", control.AckPayload{OK: true})
	_ = conn.Send(hello)

	s.extensionsMu.RLock()
	onlineHooks := append([]AgentLifecycleHook(nil), s.agentOnlineHooks...)
	s.extensionsMu.RUnlock()
	go func() {
		for _, hook := range onlineHooks {
			hook(assetID)
		}
	}()

	done := make(chan struct{})
	defer close(done)
	wsutil.StartPingLoop(ws, done, 90*time.Second, 25*time.Second, &conn.mu)

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			offlineReason = classifyControlClose(err)
			return
		}
		var env control.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "ping":
			pong, _ := control.Marshal("pong", env.RequestID, nil)
			_ = conn.Send(pong)
		case "netinfo":
			payload, err := control.UnmarshalPayload[control.NetInfoPayload](env)
			if err != nil {
				continue
			}
			_ = s.postJSON("/api/internal/agent/netinfo", map[string]any{
				"assetId":    assetID,
				"agentToken": token,
				"privateIp":  payload.PrivateIP,
			}, nil)
		default:
			s.extensionsMu.RLock()
			handler := s.controlHandlers[env.Type]
			s.extensionsMu.RUnlock()
			if handler == nil {
				continue
			}
			send := func(typ, requestID string, payload any) error {
				msg, err := control.Marshal(typ, requestID, payload)
				if err != nil {
					return err
				}
				return conn.Send(msg)
			}
			if err := handler(ControlContext{AssetID: assetID, Send: send}, env); err != nil {
				log.Printf("agent control handler type=%s asset=%s: %v", env.Type, assetID, err)
			}
		}
	}
}

func newConnectionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func classifyControlClose(err error) string {
	if err == nil {
		return "disconnected"
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "read_timeout"
	}
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived) {
		return "client_close"
	}
	if websocket.IsUnexpectedCloseError(err) {
		return "abnormal_close"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "i/o timeout") {
		return "read_timeout"
	}
	if strings.Contains(msg, "use of closed network connection") {
		return "gateway_close"
	}
	return "disconnected"
}

// clientIP returns the TCP peer address. Do not trust X-Forwarded-For -
// Agents (or anyone) can forge it; asset public/source IP must come from the
// real Gateway-facing connection.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
