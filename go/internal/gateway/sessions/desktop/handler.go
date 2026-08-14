package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/gateway/sessions/desktop/guac"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

const guacRecordingName = "session"

type Handler struct{ deps Deps }

func NewHandler(deps Deps) *Handler { return &Handler{deps: deps} }

type guacWSWriter struct {
	mu sync.Mutex
	ws *websocket.Conn
}

type guacTransportWriter interface{ write([]byte) error }

func (w *guacWSWriter) write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ws.WriteMessage(websocket.TextMessage, data)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		http.Error(w, "ticket required", http.StatusBadRequest)
		return
	}
	base, raw, err := h.deps.VerifyTicket(ticket)
	if err != nil {
		http.Error(w, "invalid ticket: "+err.Error(), http.StatusUnauthorized)
		return
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		http.Error(w, "invalid desktop claims", http.StatusBadRequest)
		return
	}
	proto := base.Type
	if proto != "rdp" && proto != "vnc" {
		http.Error(w, "not a desktop ticket", http.StatusBadRequest)
		return
	}
	if h.deps.GuacdAddr == "" {
		http.Error(w, "guacd not configured (set OPS_GUACD_ADDR)", http.StatusServiceUnavailable)
		return
	}
	targetHost := claims.TargetHost
	if targetHost == "" {
		targetHost = "127.0.0.1"
	}
	domain := ""
	if proto == "rdp" && claims.DesktopUsername != "" && !strings.ContainsAny(claims.DesktopUsername, `\@`) {
		domain = "."
	}
	bridgeHost := h.deps.GuacBridgeHost
	if bridgeHost == "" {
		bridgeHost = "127.0.0.1"
	}
	width, height, dpi := queryInt(r, "width", 320, 8192), queryInt(r, "height", 200, 8192), queryInt(r, "dpi", 48, 600)
	browserWS, err := h.deps.Upgrade(w, r)
	if err != nil {
		return
	}
	defer browserWS.Close()
	writer := &guacWSWriter{ws: browserWS}
	opType := auditstore.TypeRDP
	if proto == "vnc" {
		opType = auditstore.TypeVNC
	}
	auditDetail := map[string]any{
		"targetHost": targetHost, "targetPort": claims.TargetPort, "desktopUsername": claims.DesktopUsername,
		"desktopColorDepth": claims.DesktopColorDepth, "desktopRdpQuality": claims.DesktopRdpQuality,
	}
	recordingDir := h.recordingDir(base.SessionID)
	guacConn, agentWS, cancel, err := h.openBackend(base, ticket, proto, targetHost, claims.TargetPort,
		bridgeHost, claims.DesktopUsername, claims.DesktopPassword, domain, claims.Hostname, recordingDir,
		width, height, dpi, claims.DesktopColorDepth, claims.DesktopRdpQuality)
	if err != nil {
		log.Printf("desktop %s failed session=%s: %v", proto, base.SessionID, err)
		_ = writer.write(guac.Encode("error", err.Error(), "519"))
		h.auditEnd(opType, base, false, mergeDetail(auditDetail, map[string]any{"error": err.Error()}))
		return
	}
	defer guacConn.Close()
	defer agentWS.Close()
	defer cancel()
	if err := writer.write(guac.Encode("", guacConn.ID)); err != nil {
		return
	}
	h.auditStart(opType, base, mergeDetail(auditDetail, map[string]any{"guacId": guacConn.ID}))
	if recordingDir != "" && h.deps.AttachFinalize != nil {
		h.deps.AttachFinalize(base.SessionID, func() map[string]any { return h.recordingDetail(recordingDir) })
	}
	keepDone := make(chan struct{})
	defer close(keepDone)
	sessioncore.StartKeepAlive(browserWS, keepDone)
	sessioncore.StartKeepAlive(agentWS, keepDone)
	errCh := make(chan error, 2)
	go func() { errCh <- relayGuacdToBrowser(writer, guacConn) }()
	go func() { errCh <- relayBrowserToGuacd(browserWS, writer, guacConn) }()
	<-errCh
	cancel()
	_ = browserWS.Close()
	_ = guacConn.Close()
	_ = agentWS.Close()
	h.auditEnd(opType, base, true, mergeDetail(auditDetail, h.recordingDetail(recordingDir)))
}

func (h *Handler) openBackend(base sessioncore.BaseClaims, ticket, proto, targetHost string, targetPort int,
	bridgeHost, username, password, domain, hostname, recordingDir string, width, height, dpi, colorDepth int, rdpQuality string,
) (*guac.Connection, *websocket.Conn, context.CancelFunc, error) {
	type waitResult struct {
		ws  *websocket.Conn
		err error
	}
	h.deps.Arm(base.SessionID)
	waitDone := make(chan waitResult, 1)
	go func() {
		ws, err := h.deps.Wait(base.SessionID, 20*time.Second)
		waitDone <- waitResult{ws, err}
	}()
	if err := h.deps.OpenAgentSession(base.AssetID, base.SessionID, ticket, proto,
		Params{TargetHost: targetHost, TargetPort: targetPort}); err != nil {
		return nil, nil, nil, err
	}
	wr := <-waitDone
	if wr.err != nil {
		return nil, nil, nil, wr.err
	}
	bridgeLn, bridgePort, err := listenBridge()
	if err != nil {
		_ = wr.ws.Close()
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go serveBridge(ctx, bridgeLn, wr.ws)
	conn, err := guac.DialAndHandshake(h.deps.GuacdAddr, guac.ConnParams{
		Protocol: proto, Hostname: bridgeHost, Port: bridgePort, Username: username, Password: password,
		Domain: domain, Security: security(proto), Width: width, Height: height, DPI: dpi, Name: hostname,
		RecordingPath: recordingDir, RecordingName: guacRecordingName,
		ColorDepth: colorDepth, RdpQuality: rdpQuality,
	})
	if err != nil {
		cancel()
		_ = bridgeLn.Close()
		_ = wr.ws.Close()
		return nil, nil, nil, err
	}
	return conn, wr.ws, cancel, nil
}

func security(proto string) string {
	if proto == "rdp" {
		return "nla"
	}
	return ""
}

func (h *Handler) auditStart(op string, base sessioncore.BaseClaims, detail map[string]any) {
	if h.deps.AuditStart != nil {
		h.deps.AuditStart(op, base, detail)
	}
}
func (h *Handler) auditEnd(op string, base sessioncore.BaseClaims, ok bool, detail map[string]any) {
	if h.deps.AuditEnd != nil {
		h.deps.AuditEnd(op, base, ok, detail)
	}
}
func (h *Handler) recordingDetail(dir string) map[string]any {
	return recordingDetail(h.deps, dir)
}

func listenBridge() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, 0, err
	}
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

func serveBridge(ctx context.Context, ln net.Listener, agentWS *websocket.Conn) {
	_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(30 * time.Second))
	conn, err := ln.Accept()
	if err != nil {
		log.Printf("desktop bridge accept: %v", err)
		return
	}
	defer conn.Close()
	_ = ln.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	tunnel := sessionws.NewBinary(agentWS)
	defer tunnel.Close()
	_ = sessionws.Pipe(ctx, tunnel, conn)
}

func relayGuacdToBrowser(dst guacTransportWriter, src *guac.Connection) error {
	for {
		op, args, err := guac.ReadInstruction(src.Reader)
		if err != nil {
			return err
		}
		if err := dst.write(guac.Encode(op, args...)); err != nil {
			return err
		}
	}
}

func relayBrowserToGuacd(src *websocket.Conn, dst guacTransportWriter, guacd io.Writer) error {
	for {
		_, data, err := src.ReadMessage()
		if err != nil {
			return err
		}
		_ = src.SetReadDeadline(time.Now().Add(sessioncore.KeepAliveIdle))
		if err := forwardBrowserInstructions(data, dst, guacd); err != nil {
			return err
		}
	}
}

func forwardBrowserInstructions(data []byte, dst guacTransportWriter, guacd io.Writer) error {
	instructions, err := guac.ParseInstructions(data)
	if err != nil {
		return fmt.Errorf("invalid browser guacamole instruction: %w", err)
	}
	for _, inst := range instructions {
		if inst.Opcode == "" {
			if len(inst.Args) >= 2 && inst.Args[0] == "ping" {
				if err := dst.write(inst.Encode()); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := guacd.Write(inst.Encode()); err != nil {
			return err
		}
	}
	return nil
}

func queryInt(r *http.Request, key string, min, max int) int {
	var value int
	if _, err := fmt.Sscanf(r.URL.Query().Get(key), "%d", &value); err != nil || value < min || value > max {
		return 0
	}
	return value
}

func mergeDetail(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
