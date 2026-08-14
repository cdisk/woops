package portmap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

func (s *Service) openReverse(ctx context.Context, e *entry) (map[string]any, error) {
	if !s.deps.HasAgent(e.runtime.AssetID) {
		s.Manager.take(e.runtime.MappingID)
		e.cancel()
		log.Printf("portmap reverse pending (agent offline) mapping=%s asset=%s :%d",
			e.runtime.MappingID, e.runtime.AssetID, e.runtime.ListenPort)
		return map[string]any{
			"status": "ok", "listening": false, "reason": "agent_offline",
			"listenPort": e.runtime.ListenPort, "listenHost": e.runtime.ListenHost,
			"direction": dirAssetToGateway,
		}, nil
	}
	msg, err := control.Marshal("portmap_listen", e.runtime.MappingID, control.PortmapListenPayload{
		MappingID: e.runtime.MappingID, Protocol: e.runtime.Protocol,
		ListenHost: e.runtime.ListenHost, ListenPort: e.runtime.ListenPort,
		TargetHost: e.runtime.TargetHost, TargetPort: e.runtime.TargetPort,
	})
	if err != nil {
		s.close(e.runtime.MappingID)
		return nil, err
	}
	if err := s.deps.SendAgent(e.runtime.AssetID, msg); err != nil {
		s.close(e.runtime.MappingID)
		return nil, fmt.Errorf("send portmap_listen: %w", err)
	}
	select {
	case <-ctx.Done():
		s.close(e.runtime.MappingID)
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		s.close(e.runtime.MappingID)
		return nil, fmt.Errorf("timeout waiting agent listen status")
	case st := <-e.statusCh:
		if !st.OK {
			s.close(e.runtime.MappingID)
			return nil, fmt.Errorf("agent listen failed: %s", st.Error)
		}
		port := st.ListenPort
		if port <= 0 {
			port = e.runtime.ListenPort
		}
		host := st.ListenHost
		if host == "" {
			host = e.runtime.ListenHost
		}
		e.setListening(true, port, "")
		log.Printf("portmap reverse listening %s:%d → %s:%d mapping=%s asset=%s proto=%s",
			host, port, e.runtime.TargetHost, e.runtime.TargetPort,
			e.runtime.MappingID, e.runtime.AssetID, e.runtime.Protocol)
		return map[string]any{
			"status": "ok", "listening": true,
			"listenPort": port, "listenHost": host, "direction": dirAssetToGateway,
		}, nil
	}
}

// HandleAgentOpen authenticates the Agent, mints a reverse tunnel ticket,
// Arms pending, then serves the data WSS when it arrives.
func (s *Service) HandleAgentOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	assetID, token := r.Header.Get("X-Asset-Id"), r.Header.Get("X-Agent-Token")
	if assetID == "" || token == "" {
		http.Error(w, "missing agent credentials", http.StatusUnauthorized)
		return
	}
	if err := s.deps.PostJSON("/api/internal/agent/auth", map[string]any{
		"assetId": assetID, "agentToken": token,
	}, &map[string]any{}); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.deps.HasAgent(assetID) {
		http.Error(w, "agent control not connected", http.StatusConflict)
		return
	}
	var req struct {
		MappingID  string `json:"mappingId"`
		ClientAddr string `json:"clientAddr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.MappingID == "" {
		http.Error(w, "mappingId required", http.StatusBadRequest)
		return
	}
	if s.handleEphemeralAgentOpen(w, assetID, req.MappingID, req.ClientAddr) {
		return
	}
	e, ok := s.Manager.get(req.MappingID)
	if !ok {
		http.Error(w, "mapping not active", http.StatusNotFound)
		return
	}
	if e.runtime.AssetID != assetID {
		http.Error(w, "mapping asset mismatch", http.StatusForbidden)
		return
	}
	if e.runtime.Direction != dirAssetToGateway {
		http.Error(w, "mapping is not reverse", http.StatusBadRequest)
		return
	}

	var opened map[string]any
	if err := s.deps.PostJSON("/api/internal/port-mappings/connections/open-record", map[string]any{
		"mappingId": req.MappingID, "clientAddr": req.ClientAddr,
	}, &opened); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	sessionID, ticket, protocol := str(opened["sessionId"]), str(opened["ticket"]), str(opened["protocol"])
	if protocol == "" {
		protocol = e.runtime.Protocol
	}
	targetHost := str(opened["targetHost"])
	if targetHost == "" {
		targetHost = e.runtime.TargetHost
	}
	targetPort := intFrom(opened["targetPort"], e.runtime.TargetPort)
	if sessionID == "" || ticket == "" {
		http.Error(w, "open-record incomplete", http.StatusBadGateway)
		return
	}
	s.deps.Arm(sessionID)
	go s.serveReverseSession(e, sessionID, protocol, targetHost, targetPort, req.ClientAddr)
	writeJSON(w, map[string]any{
		"sessionId": sessionID, "ticket": ticket, "protocol": protocol,
		"direction": dirAssetToGateway, "targetHost": targetHost,
		"targetPort": targetPort, "mappingId": req.MappingID,
	})
}

func (s *Service) serveReverseSession(e *entry, sessionID, protocol, targetHost string,
	targetPort int, clientAddr string) {
	ws, err := s.deps.Wait(sessionID, 20*time.Second)
	if err != nil {
		log.Printf("portmap reverse wait failed mapping=%s session=%s: %v", e.runtime.MappingID, sessionID, err)
		opType := auditstore.TypePortmapTCP
		if protocol == "udp" {
			opType = auditstore.TypePortmapUDP
		}
		s.deps.AuditEnd(opType, sessionID, e.runtime.AssetID, false,
			mergeDetail(e.auditDetail(clientAddr), map[string]any{"error": err.Error()}))
		return
	}
	defer ws.Close()
	if protocol == "udp" {
		s.serveReverseUDP(e, sessionID, ws, targetHost, targetPort, clientAddr)
		return
	}
	s.serveReverseTCP(e, sessionID, ws, targetHost, targetPort, clientAddr)
}

func (s *Service) serveReverseTCP(e *entry, sessionID string, agentWS *websocket.Conn,
	targetHost string, targetPort int, clientAddr string) {
	detail := e.auditDetail(clientAddr)
	target := net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort))
	remote, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		log.Printf("portmap reverse dial failed mapping=%s target=%s: %v", e.runtime.MappingID, target, err)
		s.deps.AuditEnd(auditstore.TypePortmapTCP, sessionID, e.runtime.AssetID, false,
			mergeDetail(detail, map[string]any{"error": err.Error()}))
		return
	}
	defer remote.Close()
	if tc, ok := remote.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	atomic.AddInt64(&e.runtime.ActiveConns, 1)
	defer atomic.AddInt64(&e.runtime.ActiveConns, -1)
	s.deps.AuditStart(auditstore.TypePortmapTCP, sessionID, e.runtime.AssetID, detail)

	tunnel := sessionws.NewBinary(agentWS)
	var bytesIn, bytesOut int64
	errCh := make(chan error, 2)
	go func() {
		n, err := copyPortMap(remote, tunnel)
		atomic.AddInt64(&e.runtime.BytesIn, n)
		atomic.AddInt64(&bytesIn, n)
		errCh <- err
	}()
	go func() {
		n, err := copyPortMap(tunnel, remote)
		atomic.AddInt64(&e.runtime.BytesOut, n)
		atomic.AddInt64(&bytesOut, n)
		errCh <- err
	}()
	<-errCh
	timer := time.NewTimer(time.Second)
	select {
	case <-errCh:
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	_ = remote.Close()
	_ = agentWS.Close()
	inTotal, outTotal := atomic.LoadInt64(&bytesIn), atomic.LoadInt64(&bytesOut)
	log.Printf("portmap reverse tcp closed mapping=%s client=%s bytesIn=%d bytesOut=%d",
		e.runtime.MappingID, clientAddr, inTotal, outTotal)
	s.deps.AuditEnd(auditstore.TypePortmapTCP, sessionID, e.runtime.AssetID, true,
		mergeDetail(detail, map[string]any{"bytesIn": inTotal, "bytesOut": outTotal}))
}

func (s *Service) serveReverseUDP(e *entry, sessionID string, agentWS *websocket.Conn,
	targetHost string, targetPort int, clientAddr string) {
	if clientAddr == "" {
		clientAddr = "udp-mapping"
	}
	detail := e.auditDetail(clientAddr)
	target := net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort))
	raddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		s.deps.AuditEnd(auditstore.TypePortmapUDP, sessionID, e.runtime.AssetID, false,
			mergeDetail(detail, map[string]any{"error": err.Error()}))
		return
	}
	atomic.StoreInt64(&e.runtime.ActiveConns, 1)
	defer atomic.StoreInt64(&e.runtime.ActiveConns, 0)
	s.deps.AuditStart(auditstore.TypePortmapUDP, sessionID, e.runtime.AssetID, detail)

	type clientSock struct {
		conn     *net.UDPConn
		lastSeen time.Time
	}
	var (
		mu                sync.Mutex
		clients           = map[string]*clientSock{}
		bytesIn, bytesOut int64
	)
	closeAll := func() {
		mu.Lock()
		defer mu.Unlock()
		for key, client := range clients {
			_ = client.conn.Close()
			delete(clients, key)
		}
	}
	defer closeAll()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-2 * time.Minute)
				mu.Lock()
				for key, client := range clients {
					if client.lastSeen.Before(cutoff) {
						_ = client.conn.Close()
						delete(clients, key)
					}
				}
				mu.Unlock()
			}
		}
	}()

	getClient := func(key string) (*net.UDPConn, error) {
		mu.Lock()
		defer mu.Unlock()
		if client, ok := clients[key]; ok {
			client.lastSeen = time.Now()
			return client.conn, nil
		}
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			return nil, err
		}
		clients[key] = &clientSock{conn: conn, lastSeen: time.Now()}
		go func() {
			buf := make([]byte, 64*1024)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					cancel()
					return
				}
				host, portStr, _ := net.SplitHostPort(key)
				port := 0
				fmt.Sscanf(portStr, "%d", &port)
				if err := agentWS.WriteMessage(websocket.BinaryMessage,
					datagram.Encode(host, port, buf[:n])); err != nil {
					cancel()
					return
				}
				atomic.AddInt64(&e.runtime.BytesOut, int64(n))
				atomic.AddInt64(&bytesOut, int64(n))
				mu.Lock()
				if client, ok := clients[key]; ok {
					client.lastSeen = time.Now()
				}
				mu.Unlock()
			}
		}()
		return conn, nil
	}

	errCh := make(chan error, 1)
	go func() {
		for {
			_, data, err := agentWS.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			host, port, payload, err := datagram.Decode(data)
			if err != nil {
				continue
			}
			key := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			conn, err := getClient(key)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := conn.Write(payload); err != nil {
				errCh <- err
				return
			}
			atomic.AddInt64(&e.runtime.BytesIn, int64(len(payload)))
			atomic.AddInt64(&bytesIn, int64(len(payload)))
		}
	}()
	select {
	case <-ctx.Done():
	case <-errCh:
	}
	_ = agentWS.Close()
	s.deps.AuditEnd(auditstore.TypePortmapUDP, sessionID, e.runtime.AssetID, true,
		mergeDetail(detail, map[string]any{
			"bytesIn": atomic.LoadInt64(&bytesIn), "bytesOut": atomic.LoadInt64(&bytesOut),
		}))
}
