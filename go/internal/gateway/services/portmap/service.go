package portmap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type Deps struct {
	PostJSON         func(string, any, any) error
	GetJSON          func(string, any) error
	HasAgent         func(string) bool
	SendAgent        func(string, []byte) error
	Arm              func(string)
	Wait             func(string, time.Duration) (*websocket.Conn, error)
	OpenSession      func(string, string, string, string, string, int) error
	ServeBridge      sessioncore.BridgeServeFunc
	VerifyTicket     func(string) (sessioncore.BaseClaims, json.RawMessage, error)
	UpgradeWS        func(http.ResponseWriter, *http.Request) (*websocket.Conn, error)
	AuditStart       func(string, string, string, map[string]any)
	AuditEnd         func(string, string, string, bool, map[string]any)
	AuditStartClaims func(string, sessioncore.BaseClaims, map[string]any)
	AuditEndClaims   func(string, sessioncore.BaseClaims, bool, map[string]any)
}

type Service struct {
	Manager         *Manager
	deps            Deps
	ephemeralMu     sync.Mutex
	ephemeral       map[string]*ephemeralEntry
	reverseSessions map[string]*ephemeralSession
}

func NewService(deps Deps) *Service {
	return &Service{
		Manager: NewManager(), deps: deps,
		ephemeral:       map[string]*ephemeralEntry{},
		reverseSessions: map[string]*ephemeralSession{},
	}
}

func (s *Service) HandleListeningPorts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ports": s.Manager.listeningPorts()})
}

func (s *Service) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"items": s.Manager.list()})
}

func (s *Service) HandleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MappingID  string `json:"mappingId"`
		AssetID    string `json:"assetId"`
		Direction  string `json:"direction"`
		Protocol   string `json:"protocol"`
		TargetHost string `json:"targetHost"`
		TargetPort int    `json:"targetPort"`
		ListenHost string `json:"listenHost"`
		ListenPort int    `json:"listenPort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	result, err := s.open(req.MappingID, req.AssetID, req.Direction, req.Protocol,
		req.TargetHost, req.TargetPort, req.ListenHost, req.ListenPort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, result)
}

func (s *Service) HandleClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MappingID string `json:"mappingId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	s.close(req.MappingID)
	writeJSON(w, map[string]any{"status": "ok"})
}

func normalizeDirection(d string) string {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case "", "forward", dirGatewayToAsset:
		return dirGatewayToAsset
	case "reverse", dirAssetToGateway:
		return dirAssetToGateway
	default:
		return dirGatewayToAsset
	}
}

func (s *Service) open(mappingID, assetID, direction, protocol, targetHost string,
	targetPort int, listenHost string, listenPort int) (map[string]any, error) {
	if mappingID == "" || assetID == "" || listenPort <= 0 {
		return nil, fmt.Errorf("mappingId, assetId, listenPort required")
	}
	protocol = strings.ToLower(protocol)
	if protocol != "tcp" && protocol != "udp" {
		return nil, fmt.Errorf("protocol must be tcp or udp")
	}
	direction = normalizeDirection(direction)
	if targetHost == "" {
		targetHost = "127.0.0.1"
	}
	if listenHost == "" {
		if direction == dirAssetToGateway {
			listenHost = "127.0.0.1"
		} else {
			listenHost = "0.0.0.0"
		}
	}
	if s.Manager.has(mappingID) {
		e, _ := s.Manager.get(mappingID)
		snap := e.snapshot()
		return map[string]any{"status": "ok", "listening": snap.Listening,
			"listenPort": snap.ListenPort, "listenHost": snap.ListenHost, "direction": snap.Direction}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	e := &entry{runtime: runtime{
		MappingID: mappingID, AssetID: assetID, Direction: direction, Protocol: protocol,
		TargetHost: targetHost, TargetPort: targetPort, ListenHost: listenHost,
		ListenPort: listenPort, CreatedAt: time.Now().UTC(),
	}, cancel: cancel, statusCh: make(chan control.PortmapListenStatusPayload, 1)}
	if direction == dirAssetToGateway {
		s.Manager.put(mappingID, e)
		return s.openReverse(ctx, e)
	}

	addr := net.JoinHostPort(listenHost, fmt.Sprintf("%d", listenPort))
	if protocol == "tcp" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("listen tcp %s: %w", addr, err)
		}
		e.ln, e.runtime.Listening = ln, true
		s.Manager.put(mappingID, e)
		go s.serveTCP(ctx, e)
		log.Printf("portmap tcp listen %s → %s:%d mapping=%s asset=%s", addr, targetHost, targetPort, mappingID, assetID)
	} else {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("listen udp %s: %w", addr, err)
		}
		e.pc, e.runtime.Listening = pc, true
		s.Manager.put(mappingID, e)
		go s.serveUDP(ctx, e)
		log.Printf("portmap udp listen %s → %s:%d mapping=%s asset=%s", addr, targetHost, targetPort, mappingID, assetID)
	}
	return map[string]any{"status": "ok", "listening": true,
		"listenPort": listenPort, "listenHost": listenHost, "direction": direction}, nil
}

func (s *Service) close(mappingID string) {
	e, ok := s.Manager.take(mappingID)
	if !ok {
		return
	}
	if e.runtime.Direction == dirAssetToGateway && s.deps.HasAgent(e.runtime.AssetID) {
		msg, err := control.Marshal("portmap_unlisten", mappingID,
			control.PortmapUnlistenPayload{MappingID: mappingID})
		if err == nil {
			_ = s.deps.SendAgent(e.runtime.AssetID, msg)
		}
	}
	e.cancel()
	if e.ln != nil {
		_ = e.ln.Close()
	}
	if e.pc != nil {
		_ = e.pc.Close()
	}
	log.Printf("portmap closed mapping=%s", mappingID)
}

func (s *Service) DropReverseForAsset(assetID string) {
	s.Manager.mu.Lock()
	var ids []string
	for id, e := range s.Manager.byID {
		if e.runtime.AssetID == assetID && e.runtime.Direction == dirAssetToGateway {
			ids = append(ids, id)
		}
	}
	s.Manager.mu.Unlock()
	for _, id := range ids {
		if e, ok := s.Manager.take(id); ok {
			e.cancel()
			log.Printf("portmap reverse dropped (agent offline) mapping=%s", id)
		}
	}
	s.dropEphemeralForAsset(assetID)
}

func (s *Service) HandleListenStatus(assetID string, st control.PortmapListenStatusPayload) {
	if s.handleEphemeralListenStatus(assetID, st) {
		return
	}
	e, ok := s.Manager.get(st.MappingID)
	if !ok {
		return
	}
	if e.runtime.AssetID != assetID {
		log.Printf("portmap status asset mismatch mapping=%s got=%s want=%s", st.MappingID, assetID, e.runtime.AssetID)
		return
	}
	select {
	case e.statusCh <- st:
	default:
		if st.OK {
			e.setListening(true, st.ListenPort, "")
		} else {
			e.setListening(false, st.ListenPort, st.Error)
			_ = s.deps.PostJSON("/api/internal/port-mappings/last-error",
				map[string]any{"mappingId": st.MappingID, "error": st.Error}, nil)
		}
	}
}

func (s *Service) RestoreForAsset(assetID string) {
	var rows []map[string]any
	if err := s.deps.GetJSON("/api/internal/port-mappings?assetId="+assetID, &rows); err != nil {
		log.Printf("portmap restore list failed asset=%s: %v", assetID, err)
		return
	}
	for _, row := range rows {
		mappingID := str(row["mappingId"])
		if mappingID == "" {
			mappingID = str(row["id"])
		}
		if s.Manager.has(mappingID) {
			continue
		}
		_, err := s.open(mappingID, str(row["assetId"]), str(row["direction"]),
			str(row["protocol"]), str(row["targetHost"]), intFrom(row["targetPort"], 0),
			str(row["listenHost"]), intFrom(row["listenPort"], 0))
		if err != nil {
			log.Printf("portmap restore failed mapping=%s: %v", mappingID, err)
			_ = s.deps.PostJSON("/api/internal/port-mappings/last-error",
				map[string]any{"mappingId": mappingID, "error": err.Error()}, nil)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func intFrom(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if parsed, err := strconv.Atoi(n); err == nil {
			return parsed
		}
	}
	return def
}
