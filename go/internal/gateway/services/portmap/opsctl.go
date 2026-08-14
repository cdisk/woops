package portmap

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

const (
	typeOpsctlReverseControl = "opsctl_portmap_reverse"
	dirOpsctlToAsset         = "opsctl_to_asset"
	dirAssetToOpsctl         = "asset_to_opsctl"
)

type opsctlClaims struct {
	sessioncore.BaseClaims
	Ephemeral   bool   `json:"ephemeral"`
	EphemeralID string `json:"ephemeralId"`
	Direction   string `json:"direction"`
	Initiator   string `json:"initiator"`
	Protocol    string `json:"protocol"`
	ListenHost  string `json:"listenHost"`
	ListenPort  int    `json:"listenPort"`
	TargetHost  string `json:"targetHost"`
	TargetPort  int    `json:"targetPort"`
	ClientAddr  string `json:"clientAddr"`
}

func (c opsctlClaims) protocol() string {
	if c.Protocol != "" {
		return c.Protocol
	}
	return c.Type
}

func (c opsctlClaims) detail(clientAddr string) map[string]any {
	if clientAddr == "" {
		clientAddr = c.ClientAddr
	}
	return map[string]any{
		"ephemeral":  true,
		"initiator":  "opsctl",
		"direction":  c.Direction,
		"listenHost": c.ListenHost,
		"listenPort": c.ListenPort,
		"targetHost": c.TargetHost,
		"targetPort": c.TargetPort,
		"clientAddr": clientAddr,
	}
}

type reverseOpenResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	Ticket    string `json:"ticket"`
	Error     string `json:"error"`
}

type ephemeralEntry struct {
	claims    opsctlClaims
	ws        *websocket.Conn
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan reverseOpenResponse
	statusCh  chan control.PortmapListenStatusPayload
}

func (e *ephemeralEntry) writeJSON(v any) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_ = e.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return e.ws.WriteJSON(v)
}

func (e *ephemeralEntry) close() {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_ = e.ws.Close()
}

type ephemeralSession struct {
	base       sessioncore.BaseClaims
	claims     opsctlClaims
	clientAddr string
	ctlWS      chan *websocket.Conn
	done       chan struct{}
}

func ephemeralKey(assetID, ephemeralID string) string {
	return assetID + "\x00" + ephemeralID
}

// HandleOpsctlForward is the woopsctl-side adapter for the shared Agent tunnel.
// The listen socket lives in woopsctl, so Gateway only bridges two WSS peers.
func (s *Service) HandleOpsctlForward(protocol string) http.HandlerFunc {
	opType := auditstore.TypeForProtocol(protocol)
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.ServeBridge == nil {
			http.Error(w, "portmap bridge unavailable", http.StatusServiceUnavailable)
			return
		}
		s.deps.ServeBridge(sessioncore.BridgeSpec{
			ExpectedTypes: []string{protocol},
			Protocol:      protocol,
			OperationType: opType,
			Prepare: func(base sessioncore.BaseClaims, raw json.RawMessage) (sessioncore.BridgePrepared, error) {
				var claims opsctlClaims
				if err := json.Unmarshal(raw, &claims); err != nil {
					return sessioncore.BridgePrepared{}, err
				}
				if err := validateOpsctlClaims(base, claims, dirOpsctlToAsset); err != nil {
					return sessioncore.BridgePrepared{}, err
				}
				detail := claims.detail(claims.ClientAddr)
				prepared := sessioncore.BridgePrepared{
					Params:      control.TunnelParams{TargetHost: claims.TargetHost, TargetPort: claims.TargetPort},
					StartDetail: detail,
					Finish: func() (bool, map[string]any) {
						return true, detail
					},
				}
				if protocol == "udp" {
					prepared.BrowserToAgentSize = datagramPayloadSize
					prepared.AgentToBrowserSize = datagramPayloadSize
				}
				return prepared, nil
			},
		}, w, r)
	}
}

func datagramPayloadSize(messageType int, frame []byte) int64 {
	if messageType != websocket.BinaryMessage {
		return 0
	}
	_, _, payload, err := datagram.Decode(frame)
	if err != nil {
		return 0
	}
	return int64(len(payload))
}

func validateOpsctlClaims(base sessioncore.BaseClaims, claims opsctlClaims, direction string) error {
	if !claims.Ephemeral || !strings.HasPrefix(claims.EphemeralID, "opsctl:") {
		return fmt.Errorf("opsctl ephemeralId required")
	}
	if claims.Direction != direction || claims.Initiator != "opsctl" {
		return fmt.Errorf("invalid portmap direction or initiator")
	}
	if claims.TargetHost == "" || claims.TargetPort <= 0 || claims.TargetPort > 65535 {
		return fmt.Errorf("invalid target")
	}
	if claims.ListenHost == "" || claims.ListenPort <= 0 || claims.ListenPort > 65535 {
		return fmt.Errorf("invalid listen address")
	}
	if base.AssetID == "" {
		return fmt.Errorf("assetId required")
	}
	if protocol := claims.protocol(); protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("protocol must be tcp or udp")
	}
	return nil
}

// HandleOpsctlReverseControl owns the desired-state lease for an Agent-side
// listener. Closing this WSS removes the listener; woopsctl reconnects with the
// same ephemeralId after transient failures.
func (s *Service) HandleOpsctlReverseControl(w http.ResponseWriter, r *http.Request) {
	base, claims, err := s.verifyOpsctlRequest(r, typeOpsctlReverseControl, dirAssetToOpsctl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if s.Manager.has(claims.EphemeralID) {
		http.Error(w, "ephemeralId conflicts with persistent mapping", http.StatusConflict)
		return
	}
	if s.deps.UpgradeWS == nil {
		http.Error(w, "websocket unavailable", http.StatusServiceUnavailable)
		return
	}
	ws, err := s.deps.UpgradeWS(w, r)
	if err != nil {
		return
	}
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(ws, keepDone)
	entry := &ephemeralEntry{
		claims: claims, ws: ws,
		pending:  make(map[string]chan reverseOpenResponse),
		statusCh: make(chan control.PortmapListenStatusPayload, 1),
	}
	s.ephemeralMu.Lock()
	key := ephemeralKey(base.AssetID, claims.EphemeralID)
	old := s.ephemeral[key]
	s.ephemeral[key] = entry
	s.ephemeralMu.Unlock()
	if old != nil {
		old.close()
	}
	defer s.removeEphemeral(entry)

	msg, err := control.Marshal("portmap_listen", claims.EphemeralID, control.PortmapListenPayload{
		MappingID:  claims.EphemeralID,
		Protocol:   claims.protocol(),
		ListenHost: claims.ListenHost,
		ListenPort: claims.ListenPort,
		TargetHost: claims.TargetHost,
		TargetPort: claims.TargetPort,
	})
	if err != nil {
		_ = entry.writeJSON(map[string]any{"type": "status", "ok": false, "error": err.Error()})
		return
	}
	if err := s.deps.SendAgent(base.AssetID, msg); err != nil {
		_ = entry.writeJSON(map[string]any{"type": "status", "ok": false, "error": err.Error()})
		return
	}

	select {
	case status := <-entry.statusCh:
		_ = entry.writeJSON(map[string]any{
			"type": "status", "ok": status.OK, "error": status.Error,
			"listenHost": status.ListenHost, "listenPort": status.ListenPort,
		})
		if !status.OK {
			return
		}
	case <-time.After(15 * time.Second):
		_ = entry.writeJSON(map[string]any{"type": "status", "ok": false, "error": "timeout waiting agent listener"})
		return
	}

	for {
		var response reverseOpenResponse
		if err := ws.ReadJSON(&response); err != nil {
			return
		}
		if response.Type != "open_response" || response.RequestID == "" {
			continue
		}
		entry.pendingMu.Lock()
		ch := entry.pending[response.RequestID]
		entry.pendingMu.Unlock()
		if ch != nil {
			select {
			case ch <- response:
			default:
			}
		}
	}
}

func (s *Service) verifyOpsctlRequest(r *http.Request, expectedType, direction string) (sessioncore.BaseClaims, opsctlClaims, error) {
	if s.deps.VerifyTicket == nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("ticket verifier unavailable")
	}
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("ticket required")
	}
	base, raw, err := s.deps.VerifyTicket(ticket)
	if err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("invalid ticket: %w", err)
	}
	if expectedType != "" && base.Type != expectedType {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("ticket type mismatch")
	}
	var claims opsctlClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, err
	}
	if err := validateOpsctlClaims(base, claims, direction); err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, err
	}
	return base, claims, nil
}

func (s *Service) handleEphemeralListenStatus(assetID string, status control.PortmapListenStatusPayload) bool {
	s.ephemeralMu.Lock()
	entry := s.ephemeral[ephemeralKey(assetID, status.MappingID)]
	s.ephemeralMu.Unlock()
	if entry == nil {
		return false
	}
	if entry.claims.AssetID != assetID {
		return true
	}
	select {
	case entry.statusCh <- status:
	default:
	}
	return true
}

func (s *Service) removeEphemeral(entry *ephemeralEntry) {
	s.ephemeralMu.Lock()
	key := ephemeralKey(entry.claims.AssetID, entry.claims.EphemeralID)
	owned := s.ephemeral[key] == entry
	if owned {
		delete(s.ephemeral, key)
	}
	s.ephemeralMu.Unlock()
	if owned && s.deps.HasAgent(entry.claims.AssetID) {
		if msg, err := control.Marshal("portmap_unlisten", entry.claims.EphemeralID,
			control.PortmapUnlistenPayload{MappingID: entry.claims.EphemeralID}); err == nil {
			_ = s.deps.SendAgent(entry.claims.AssetID, msg)
		}
	}
	entry.close()
}

func (s *Service) dropEphemeralForAsset(assetID string) {
	s.ephemeralMu.Lock()
	var entries []*ephemeralEntry
	for _, entry := range s.ephemeral {
		if entry.claims.AssetID == assetID {
			entries = append(entries, entry)
		}
	}
	s.ephemeralMu.Unlock()
	for _, entry := range entries {
		entry.close()
	}
}

// handleEphemeralAgentOpen asks the owning woopsctl process to mint a
// per-connection ticket, then returns that same ticket to the Agent.
func (s *Service) handleEphemeralAgentOpen(w http.ResponseWriter, assetID, mappingID, clientAddr string) bool {
	s.ephemeralMu.Lock()
	entry := s.ephemeral[ephemeralKey(assetID, mappingID)]
	s.ephemeralMu.Unlock()
	if entry == nil {
		return false
	}
	if entry.claims.AssetID != assetID {
		http.Error(w, "mapping asset mismatch", http.StatusForbidden)
		return true
	}
	opened := false
	defer func() {
		if !opened {
			// TCP can accept again, but UDP creates its only data session here.
			// Re-establishing the control lease is the one recovery path that is
			// correct for both protocols.
			entry.close()
		}
	}()

	requestID, err := randomID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	responseCh := make(chan reverseOpenResponse, 1)
	entry.pendingMu.Lock()
	entry.pending[requestID] = responseCh
	entry.pendingMu.Unlock()
	defer func() {
		entry.pendingMu.Lock()
		delete(entry.pending, requestID)
		entry.pendingMu.Unlock()
	}()

	if err := entry.writeJSON(map[string]any{
		"type": "open_request", "requestId": requestID,
		"clientAddr": clientAddr, "protocol": entry.claims.protocol(),
	}); err != nil {
		http.Error(w, "woopsctl control disconnected", http.StatusBadGateway)
		return true
	}

	var response reverseOpenResponse
	select {
	case response = <-responseCh:
	case <-time.After(20 * time.Second):
		http.Error(w, "timeout waiting woopsctl ticket", http.StatusGatewayTimeout)
		return true
	}
	if response.Error != "" {
		http.Error(w, response.Error, http.StatusBadGateway)
		return true
	}
	base, claims, err := s.verifyReverseConnectionTicket(response.Ticket, response.SessionID, entry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return true
	}

	session := &ephemeralSession{
		base: base, claims: claims, clientAddr: clientAddr,
		ctlWS: make(chan *websocket.Conn, 1),
		done:  make(chan struct{}),
	}
	s.ephemeralMu.Lock()
	s.reverseSessions[base.SessionID] = session
	s.ephemeralMu.Unlock()
	s.deps.Arm(base.SessionID)
	go s.serveEphemeralReverse(session)
	writeJSON(w, map[string]any{
		"sessionId": base.SessionID, "ticket": response.Ticket,
		"protocol": base.Type, "direction": dirAssetToOpsctl,
		"ephemeralId": mappingID,
	})
	opened = true
	return true
}

func (s *Service) verifyReverseConnectionTicket(ticket, sessionID string, entry *ephemeralEntry) (sessioncore.BaseClaims, opsctlClaims, error) {
	if ticket == "" || s.deps.VerifyTicket == nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("reverse ticket required")
	}
	base, raw, err := s.deps.VerifyTicket(ticket)
	if err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, err
	}
	if base.SessionID != sessionID || base.AssetID != entry.claims.AssetID ||
		(base.Type != "tcp" && base.Type != "udp") || base.Type != entry.claims.protocol() {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("reverse ticket mismatch")
	}
	var claims opsctlClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, err
	}
	if err := validateOpsctlClaims(base, claims, dirAssetToOpsctl); err != nil {
		return sessioncore.BaseClaims{}, opsctlClaims{}, err
	}
	if claims.EphemeralID != entry.claims.EphemeralID {
		return sessioncore.BaseClaims{}, opsctlClaims{}, fmt.Errorf("ephemeralId mismatch")
	}
	return base, claims, nil
}

func (s *Service) HandleOpsctlReverseData(w http.ResponseWriter, r *http.Request) {
	base, claims, err := s.verifyOpsctlRequest(r, "", dirAssetToOpsctl)
	if err != nil || (base.Type != "tcp" && base.Type != "udp") {
		if err == nil {
			err = fmt.Errorf("ticket type mismatch")
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var session *ephemeralSession
	deadline := time.Now().Add(5 * time.Second)
	for session == nil && time.Now().Before(deadline) {
		s.ephemeralMu.Lock()
		session = s.reverseSessions[base.SessionID]
		s.ephemeralMu.Unlock()
		if session == nil {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if session == nil || session.claims.EphemeralID != claims.EphemeralID {
		http.Error(w, "reverse session not pending", http.StatusNotFound)
		return
	}
	ws, err := s.deps.UpgradeWS(w, r)
	if err != nil {
		return
	}
	select {
	case session.ctlWS <- ws:
	case <-session.done:
		_ = ws.Close()
	default:
		_ = ws.Close()
	}
}

func (s *Service) serveEphemeralReverse(session *ephemeralSession) {
	defer func() {
		close(session.done)
		s.ephemeralMu.Lock()
		delete(s.reverseSessions, session.base.SessionID)
		entry := s.ephemeral[ephemeralKey(session.claims.AssetID, session.claims.EphemeralID)]
		s.ephemeralMu.Unlock()
		if session.base.Type == "udp" && entry != nil {
			// A reverse UDP listener has exactly one data WSS. If it dies, force
			// the desired-state client to reconnect and recreate the listener.
			entry.close()
		}
	}()
	agentWS, waitErr := s.deps.Wait(session.base.SessionID, 20*time.Second)
	if waitErr != nil {
		select {
		case ctlWS := <-session.ctlWS:
			_ = ctlWS.Close()
		default:
		}
		s.auditEphemeralEnd(session, false, map[string]any{"error": waitErr.Error()})
		return
	}
	var ctlWS *websocket.Conn
	select {
	case ctlWS = <-session.ctlWS:
	case <-time.After(20 * time.Second):
		_ = agentWS.Close()
		s.auditEphemeralEnd(session, false, map[string]any{"error": "timeout waiting woopsctl data peer"})
		return
	}
	defer agentWS.Close()
	defer ctlWS.Close()

	detail := session.claims.detail(session.clientAddr)
	opType := auditstore.TypeForProtocol(session.base.Type)
	if s.deps.AuditStartClaims != nil {
		s.deps.AuditStartClaims(opType, session.base, detail)
	}
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(agentWS, keepDone)
	sessioncore.StartKeepAlive(ctlWS, keepDone)

	var bytesIn, bytesOut int64
	if session.base.Type == "udp" {
		relayDatagramPeers(agentWS, ctlWS, &bytesIn, &bytesOut)
	} else {
		relayStreamPeers(agentWS, ctlWS, &bytesIn, &bytesOut)
	}
	s.auditEphemeralEnd(session, true, map[string]any{
		"bytesIn": atomic.LoadInt64(&bytesIn), "bytesOut": atomic.LoadInt64(&bytesOut),
	})
}

func relayStreamPeers(agentWS, ctlWS *websocket.Conn, bytesIn, bytesOut *int64) {
	agent := sessionws.NewBinary(agentWS)
	ctl := sessionws.NewBinary(ctlWS)
	errCh := make(chan error, 2)
	go func() {
		n, err := copyPortMap(ctl, agent)
		atomic.AddInt64(bytesIn, n)
		errCh <- err
	}()
	go func() {
		n, err := copyPortMap(agent, ctl)
		atomic.AddInt64(bytesOut, n)
		errCh <- err
	}()
	waitPortmapDrain(errCh)
	_ = agent.Close()
	_ = ctl.Close()
}

func relayDatagramPeers(agentWS, ctlWS *websocket.Conn, bytesIn, bytesOut *int64) {
	errCh := make(chan error, 2)
	go func() { errCh <- copyDatagramFrames(ctlWS, agentWS, bytesIn) }()
	go func() { errCh <- copyDatagramFrames(agentWS, ctlWS, bytesOut) }()
	waitPortmapDrain(errCh)
	_ = agentWS.Close()
	_ = ctlWS.Close()
}

func copyDatagramFrames(dst, src *websocket.Conn, counted *int64) error {
	for {
		messageType, frame, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		_, _, payload, err := datagram.Decode(frame)
		if err != nil {
			continue
		}
		if err := dst.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return err
		}
		atomic.AddInt64(counted, int64(len(payload)))
	}
}

func waitPortmapDrain(errCh <-chan error) {
	<-errCh
	drain := time.NewTimer(time.Second)
	select {
	case <-errCh:
	case <-drain.C:
	}
	if !drain.Stop() {
		select {
		case <-drain.C:
		default:
		}
	}
}

func (s *Service) auditEphemeralEnd(session *ephemeralSession, success bool, extra map[string]any) {
	if s.deps.AuditEndClaims == nil {
		return
	}
	s.deps.AuditEndClaims(auditstore.TypeForProtocol(session.base.Type), session.base, success,
		mergeDetail(session.claims.detail(session.clientAddr), extra))
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
