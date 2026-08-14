package portmap

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessionws"
)

// Deps wires reverse portmap to the agent Runtime without an import cycle.
type Deps struct {
	OpenConnection func(mappingID, clientAddr string) (sessionID, ticket string, err error)
	DialSessionWS  func(sessionID, ticket string) (*websocket.Conn, error)
}

type Manager struct {
	mu   sync.Mutex
	byID map[string]*reversePortMap
	deps Deps
}

type reversePortMap struct {
	mappingID  string
	protocol   string
	listenHost string
	listenPort int
	cancel     context.CancelFunc
	ln         net.Listener
	pc         net.PacketConn
}

func NewManager(deps Deps) *Manager {
	return &Manager{
		byID: map[string]*reversePortMap{},
		deps: deps,
	}
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.byID))
	for id := range m.byID {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Unlisten(id)
	}
}

func (m *Manager) Unlisten(mappingID string) {
	m.mu.Lock()
	e, ok := m.byID[mappingID]
	if ok {
		delete(m.byID, mappingID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	e.cancel()
	if e.ln != nil {
		_ = e.ln.Close()
	}
	if e.pc != nil {
		_ = e.pc.Close()
	}
	log.Printf("portmap reverse unlistened mapping=%s", mappingID)
}

func (m *Manager) Listen(p control.PortmapListenPayload, report func(control.PortmapListenStatusPayload)) {
	mappingID := strings.TrimSpace(p.MappingID)
	if mappingID == "" || p.ListenPort <= 0 {
		report(control.PortmapListenStatusPayload{
			MappingID: mappingID, OK: false, Error: "mappingId and listenPort required",
		})
		return
	}
	proto := strings.ToLower(strings.TrimSpace(p.Protocol))
	if proto != "tcp" && proto != "udp" {
		report(control.PortmapListenStatusPayload{
			MappingID: mappingID, OK: false, Error: "protocol must be tcp or udp",
		})
		return
	}
	host := strings.TrimSpace(p.ListenHost)
	if host == "" {
		host = "127.0.0.1"
	}

	m.mu.Lock()
	if old, ok := m.byID[mappingID]; ok {
		m.mu.Unlock()
		// Idempotent: already listening.
		report(control.PortmapListenStatusPayload{
			MappingID: mappingID, OK: true, ListenHost: old.listenHost, ListenPort: old.listenPort,
		})
		return
	}
	m.mu.Unlock()

	addr := net.JoinHostPort(host, strconv.Itoa(p.ListenPort))
	ctx, cancel := context.WithCancel(context.Background())
	entry := &reversePortMap{
		mappingID:  mappingID,
		protocol:   proto,
		listenHost: host,
		listenPort: p.ListenPort,
		cancel:     cancel,
	}

	if proto == "tcp" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			cancel()
			report(control.PortmapListenStatusPayload{
				MappingID: mappingID, OK: false, Error: err.Error(),
			})
			return
		}
		entry.ln = ln
		if ta, ok := ln.Addr().(*net.TCPAddr); ok && ta.Port > 0 {
			entry.listenPort = ta.Port
		}
		m.mu.Lock()
		m.byID[mappingID] = entry
		m.mu.Unlock()
		report(control.PortmapListenStatusPayload{
			MappingID: mappingID, OK: true, ListenHost: host, ListenPort: entry.listenPort,
		})
		go m.serveTCP(ctx, entry)
		log.Printf("portmap reverse tcp listen %s mapping=%s", addr, mappingID)
		return
	}

	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		cancel()
		report(control.PortmapListenStatusPayload{
			MappingID: mappingID, OK: false, Error: err.Error(),
		})
		return
	}
	entry.pc = pc
	if ua, ok := pc.LocalAddr().(*net.UDPAddr); ok && ua.Port > 0 {
		entry.listenPort = ua.Port
	}
	m.mu.Lock()
	m.byID[mappingID] = entry
	m.mu.Unlock()
	report(control.PortmapListenStatusPayload{
		MappingID: mappingID, OK: true, ListenHost: host, ListenPort: entry.listenPort,
	})
	go m.serveUDP(ctx, entry)
	log.Printf("portmap reverse udp listen %s mapping=%s", addr, mappingID)
}

func (m *Manager) serveTCP(ctx context.Context, e *reversePortMap) {
	for {
		conn, err := e.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("portmap reverse accept mapping=%s: %v", e.mappingID, err)
				return
			}
		}
		go m.handleTCPConn(ctx, e, conn)
	}
}

func (m *Manager) handleTCPConn(ctx context.Context, e *reversePortMap, local net.Conn) {
	defer local.Close()
	clientAddr := local.RemoteAddr().String()
	sessionID, ticket, err := m.deps.OpenConnection(e.mappingID, clientAddr)
	if err != nil {
		log.Printf("portmap reverse open-connection mapping=%s: %v", e.mappingID, err)
		return
	}
	ws, err := m.deps.DialSessionWS(sessionID, ticket)
	if err != nil {
		log.Printf("portmap reverse dial session mapping=%s: %v", e.mappingID, err)
		return
	}
	defer ws.Close()

	tunnel := sessionws.NewBinary(ws)
	pipeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_ = sessionws.Pipe(pipeCtx, tunnel, local)
}

func (m *Manager) serveUDP(ctx context.Context, e *reversePortMap) {
	sessionID, ticket, err := m.deps.OpenConnection(e.mappingID, "udp-mapping")
	if err != nil {
		log.Printf("portmap reverse udp open-connection mapping=%s: %v", e.mappingID, err)
		m.Unlisten(e.mappingID)
		return
	}
	ws, err := m.deps.DialSessionWS(sessionID, ticket)
	if err != nil {
		log.Printf("portmap reverse udp dial session mapping=%s: %v", e.mappingID, err)
		m.Unlisten(e.mappingID)
		return
	}
	defer ws.Close()

	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := e.pc.ReadFrom(buf)
			if err != nil {
				errCh <- err
				return
			}
			host, portStr, _ := net.SplitHostPort(addr.String())
			port := 0
			fmt.Sscanf(portStr, "%d", &port)
			frame := datagram.Encode(host, port, buf[:n])
			if werr := ws.WriteMessage(websocket.BinaryMessage, frame); werr != nil {
				errCh <- werr
				return
			}
		}
	}()
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			host, port, payload, err := datagram.Decode(data)
			if err != nil {
				continue
			}
			raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				continue
			}
			if _, err := e.pc.WriteTo(payload, raddr); err != nil {
				errCh <- err
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-errCh:
	}
	_ = ws.Close()
}
