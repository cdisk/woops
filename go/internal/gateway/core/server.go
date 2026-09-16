package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

var _ Router = (*Server)(nil)

type Config struct {
	// PublicListenAddr is the external listener (HTTPS when TLS cert/key set), e.g. ":9200".
	PublicListenAddr string
	// InternalListenAddr is the plain-HTTP listener for control-api → gateway, e.g. ":9201".
	InternalListenAddr string
	// ControlInternalHTTP is Gateway → control-api base (private HTTP), e.g. http://control-api:9100.
	ControlInternalHTTP string
	// PublicHTTPBase is the external Gateway base written into install scripts / agents (https://…).
	PublicHTTPBase string
	// AgentBinDir holds packaged agent binaries named woops-agent-{os}-{arch}.
	AgentBinDir string
	// GuacdAddr is host:port of guacd (RDP/VNC decoder). Empty disables /ws/desktop.
	GuacdAddr string
	// GuacBridgeHost is the hostname guacd uses to reach the gateway's local TCP bridge.
	// Use 127.0.0.1 when guacd runs on the same host; host.docker.internal when guacd is in Docker.
	GuacBridgeHost string
	// TLSSpkiSHA256 is optional Gateway cert SPKI pin (hex) embedded into install scripts.
	TLSSpkiSHA256 string
	// AuditDir is the JSONL spool root for runtime activity. Empty disables auditing.
	AuditDir string
	// AuditInstanceID names this Gateway in segment filenames. Empty derives one.
	AuditInstanceID string
}

type Server struct {
	cfg                Config
	upgrader           websocket.Upgrader
	agents             *AgentRegistry
	pending            *sessioncore.PendingSessions
	http               *http.Client
	mux                *http.ServeMux
	internalMux        *http.ServeMux
	extensionsMu       sync.RWMutex
	controlHandlers    map[string]ControlHandler
	agentOnlineHooks   []AgentLifecycleHook
	agentOfflineHooks  []AgentLifecycleHook
	controlCloseHooks  []AgentLifecycleHook
	allowedTicketTypes map[string]struct{}
	// audit is nil when AuditDir is unset; all audit helpers are nil-safe.
	audit *auditstore.Writer
	// instanceID identifies this Gateway process in asset presence events.
	instanceID string
	// liveOps tracks START'd operations that have not yet ENDed, so SIGTERM
	// can seal them as INTERRUPTED instead of leaving RUNNING forever.
	liveOps sync.Map // operationID (string) → liveOp
}

type ControlContext struct {
	AssetID string
	Send    func(typ, requestID string, payload any) error
}

type ControlHandler func(ControlContext, control.Envelope) error
type AgentLifecycleHook func(assetID string)

type liveOp struct {
	opType string
	fields auditFields
	// finalize, when set, runs once before INTERRUPTED END (close cast / pick
	// up guac recording) so the console can still offer replay.
	finalize func() map[string]any
}

func New(cfg Config) *Server {
	if cfg.AgentBinDir == "" {
		cfg.AgentBinDir = "bin"
	}
	instanceID := auditstore.InstanceID(cfg.AuditInstanceID)
	s := &Server{
		cfg: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
			// Guacamole.WebSocketTunnel negotiates this; other clients omit it.
			Subprotocols: []string{"guacamole"},
		},
		agents:             NewAgentRegistry(),
		pending:            sessioncore.NewPendingSessions(),
		http:               &http.Client{Timeout: 10 * time.Second},
		mux:                http.NewServeMux(),
		internalMux:        http.NewServeMux(),
		instanceID:         instanceID,
		controlHandlers:    make(map[string]ControlHandler),
		allowedTicketTypes: make(map[string]struct{}),
	}
	if dir := strings.TrimSpace(cfg.AuditDir); dir != "" {
		w, err := auditstore.NewWriter(dir, cfg.AuditInstanceID)
		if err != nil {
			log.Printf("ops-audit spool disabled (%s): %v", dir, err)
		} else {
			s.audit = w
			log.Printf("ops-audit spool at %s", w.Dir())
		}
	}
	s.registerCoreRoutes()
	// nginx proxies /ws/, /i/ and /bin/ through the internal listener, so it must
	// also serve every public route. Specific internal patterns still win.
	s.internalMux.Handle("/", s.mux)
	return s
}

// CloseAudit seals open operations as INTERRUPTED, then seals the active spool
// segment. Safe when auditing is disabled.
func (s *Server) CloseAudit() error {
	s.interruptLiveOps("gateway_shutdown")
	if s.audit == nil {
		return nil
	}
	return s.audit.Close()
}

func (s *Server) registerCoreRoutes() {
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok", "agents": s.agents.Count()})
	})
	// Public agent register only - control-api stays off the public network.
	s.mux.HandleFunc("/api/agent/register", s.handleAgentRegisterProxy)
	s.mux.HandleFunc("/api/opsctl/tickets", s.handleOpsctlTicketsProxy)
	s.mux.HandleFunc("/ws/agent/control", s.handleAgentControl)
	s.mux.HandleFunc("/ws/agent/session", s.handleAgentSession)
}

// Register attaches an application-owned module route without exposing the mux.
func (s *Server) Register(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// RegisterInternal attaches a control-plane-only route. These handlers carry no
// authentication of their own: control-api is the only caller, so they must stay
// off the public listener. Never move them to Register.
func (s *Server) RegisterInternal(pattern string, handler http.HandlerFunc) {
	s.internalMux.HandleFunc(pattern, handler)
}

// Handler returns the public mux (agents / woopsctl / browser).
func (s *Server) Handler() http.Handler { return s.mux }

// InternalHandler returns the control-api-facing mux: internal routes plus the
// public routes nginx reverse-proxies through this listener.
func (s *Server) InternalHandler() http.Handler { return s.internalMux }

func (s *Server) RegisterControlHandler(typ string, handler ControlHandler) {
	typ = strings.TrimSpace(typ)
	if typ == "" || handler == nil {
		return
	}
	s.extensionsMu.Lock()
	defer s.extensionsMu.Unlock()
	if _, exists := s.controlHandlers[typ]; exists {
		panic("duplicate gateway control handler: " + typ)
	}
	s.controlHandlers[typ] = handler
}

func (s *Server) RegisterAgentOnlineHook(hook AgentLifecycleHook) {
	if hook == nil {
		return
	}
	s.extensionsMu.Lock()
	s.agentOnlineHooks = append(s.agentOnlineHooks, hook)
	s.extensionsMu.Unlock()
}

func (s *Server) RegisterAgentOfflineHook(hook AgentLifecycleHook) {
	if hook == nil {
		return
	}
	s.extensionsMu.Lock()
	s.agentOfflineHooks = append(s.agentOfflineHooks, hook)
	s.extensionsMu.Unlock()
}

func (s *Server) RegisterControlCloseHook(hook AgentLifecycleHook) {
	if hook == nil {
		return
	}
	s.extensionsMu.Lock()
	s.controlCloseHooks = append(s.controlCloseHooks, hook)
	s.extensionsMu.Unlock()
}

func (s *Server) AllowAgentSessionTypes(types ...string) {
	s.extensionsMu.Lock()
	defer s.extensionsMu.Unlock()
	for _, typ := range types {
		if typ = strings.TrimSpace(typ); typ != "" {
			s.allowedTicketTypes[typ] = struct{}{}
		}
	}
}

// VerifyTicket validates common claims while preserving raw protocol claims.
func (s *Server) VerifyTicket(ticket string) (sessioncore.BaseClaims, json.RawMessage, error) {
	return s.verifySessionTicket(ticket)
}

// UpgradeWS upgrades a module-owned HTTP route to WebSocket.
func (s *Server) UpgradeWS(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return s.upgrader.Upgrade(w, r, nil)
}

func (s *Server) GuacdAddr() string      { return s.cfg.GuacdAddr }
func (s *Server) GuacBridgeHost() string { return s.cfg.GuacBridgeHost }

// PostJSON sends a JSON request to the control plane.
func (s *Server) PostJSON(path string, body any, out any) error {
	return s.postJSON(path, body, out)
}

func (s *Server) GetJSON(path string, out any) error { return s.getJSON(path, out) }

// InterruptOrphanOperations asks control-api to seal every OPERATION still
// RUNNING. Call once at Gateway process start: force-kill cannot run CloseAudit,
// so orphans would otherwise sit in RUNNING until the stale-hours job.
func (s *Server) InterruptOrphanOperations() {
	var out map[string]any
	err := s.postJSON("/api/internal/server-operations/interrupt-running", map[string]any{
		"reason": "gateway_restart",
	}, &out)
	if err != nil {
		log.Printf("ops-audit interrupt orphans: %v", err)
		return
	}
	n, _ := out["interrupted"].(float64)
	log.Printf("ops-audit interrupt orphans: interrupted=%v reason=%v", n, out["reason"])
}

func (s *Server) HasAgent(assetID string) bool {
	_, ok := s.agents.Get(assetID)
	return ok
}

func (s *Server) SendAgent(assetID string, msg []byte) error {
	agent, ok := s.agents.Get(assetID)
	if !ok {
		return fmt.Errorf("agent offline")
	}
	return agent.Send(msg)
}

func (s *Server) ArmSession(sessionID string) { s.pending.Arm(sessionID) }

func (s *Server) WaitSession(sessionID string, timeout time.Duration) (*websocket.Conn, error) {
	return s.pending.Wait(sessionID, timeout)
}

func (s *Server) OperationStart(opType, operationID, assetID string, detail map[string]any) {
	s.auditStart(opType, auditFields{OperationID: operationID, AssetID: assetID}, detail)
}

func (s *Server) OperationEnd(opType, operationID, assetID string, success bool, detail map[string]any) {
	s.auditEnd(opType, auditFields{OperationID: operationID, AssetID: assetID}, success, "", detail)
}

// RecordingRoot exposes only the spool root needed by feature-owned recording
// policy. An empty value means recording storage is disabled.
func (s *Server) RecordingRoot() string {
	if s.audit == nil {
		return ""
	}
	return s.audit.Dir()
}

// CreateRecordingDir allocates an operation recording directory.
func (s *Server) CreateRecordingDir(operationID string) (string, error) {
	if s.audit == nil {
		return "", nil
	}
	return s.audit.RecordingDir(operationID)
}

// RecordingRelPath maps a recording file to its portable spool-relative path.
func (s *Server) RecordingRelPath(path string) string {
	if s.audit == nil {
		return ""
	}
	return s.audit.RelPath(path)
}

func (s *Server) postJSON(path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := s.http.Post(s.cfg.ControlInternalHTTP+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("control-api %s: %s", resp.Status, string(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (s *Server) getJSON(path string, out any) error {
	resp, err := s.http.Get(s.cfg.ControlInternalHTTP + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("control-api %s: %s", resp.Status, string(data))
	}
	return json.Unmarshal(data, out)
}

// handleAgentRegisterProxy forwards public install registration to private control-api.
func (s *Server) handleAgentRegisterProxy(w http.ResponseWriter, r *http.Request) {
	s.proxyControlAPI(w, r, "/api/agent/register", false)
}

// handleOpsctlTicketsProxy forwards CI ticket minting (Authorization: Bearer deploy token).
func (s *Server) handleOpsctlTicketsProxy(w http.ResponseWriter, r *http.Request) {
	s.proxyControlAPI(w, r, "/api/opsctl/tickets", true)
}

func (s *Server) proxyControlAPI(w http.ResponseWriter, r *http.Request, apiPath string, forwardAuth bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	url := strings.TrimRight(s.cfg.ControlInternalHTTP, "/") + apiPath
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if forwardAuth {
		if auth := r.Header.Get("Authorization"); auth != "" {
			req.Header.Set("Authorization", auth)
		}
	}
	resp, err := s.http.Do(req)
	if err != nil {
		http.Error(w, "control-api unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type AgentConn struct {
	AssetID string
	WS      *websocket.Conn
	PeerIP  string
	ConnID  string
	mu      sync.Mutex
}

func (a *AgentConn) Send(msg []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.WS.WriteMessage(websocket.TextMessage, msg)
}

type AgentRegistry struct {
	mu   sync.RWMutex
	byID map[string]*AgentConn
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{byID: map[string]*AgentConn{}}
}

func (r *AgentRegistry) Put(c *AgentConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.byID[c.AssetID]; ok {
		_ = old.WS.Close()
	}
	r.byID[c.AssetID] = c
}

// Remove drops c only when it is still the registered connection for assetID.
// Returns true when this was the live connection (caller should report offline).
// A superseded reconnect must not report offline after a newer conn is Put.
func (r *AgentRegistry) Remove(assetID string, c *AgentConn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.byID[assetID]; ok && cur == c {
		delete(r.byID, assetID)
		return true
	}
	return false
}

func (r *AgentRegistry) Get(assetID string) (*AgentConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[assetID]
	return c, ok
}

func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}
