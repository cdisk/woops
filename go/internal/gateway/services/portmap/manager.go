package portmap

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

const (
	dirGatewayToAsset = "gateway_to_asset"
	dirAssetToGateway = "asset_to_gateway"
)

type runtime struct {
	MappingID   string    `json:"mappingId"`
	AssetID     string    `json:"assetId"`
	Direction   string    `json:"direction"`
	Protocol    string    `json:"protocol"`
	TargetHost  string    `json:"targetHost"`
	TargetPort  int       `json:"targetPort"`
	ListenHost  string    `json:"listenHost"`
	ListenPort  int       `json:"listenPort"`
	Listening   bool      `json:"listening"`
	LastError   string    `json:"lastError,omitempty"`
	BytesIn     int64     `json:"bytesIn"`
	BytesOut    int64     `json:"bytesOut"`
	ActiveConns int64     `json:"activeConns"`
	CreatedAt   time.Time `json:"createdAt"`
}

type entry struct {
	runtime  runtime
	cancel   context.CancelFunc
	ln       net.Listener
	pc       net.PacketConn
	mu       sync.Mutex
	statusCh chan control.PortmapListenStatusPayload
}

type Manager struct {
	mu   sync.Mutex
	byID map[string]*entry
}

func NewManager() *Manager {
	return &Manager{byID: map[string]*entry{}}
}

func (m *Manager) list() []runtime {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]runtime, 0, len(m.byID))
	for _, e := range m.byID {
		out = append(out, e.snapshot())
	}
	return out
}

func (m *Manager) listeningPorts() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]int, 0, len(m.byID))
	seen := map[int]bool{}
	for _, e := range m.byID {
		if e.runtime.Direction == dirAssetToGateway {
			continue
		}
		p := e.runtime.ListenPort
		if p > 0 && e.runtime.Listening && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (m *Manager) has(mappingID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.byID[mappingID]
	return ok
}

func (m *Manager) get(mappingID string) (*entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[mappingID]
	return e, ok
}

func (m *Manager) put(mappingID string, e *entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[mappingID] = e
}

func (m *Manager) take(mappingID string) (*entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[mappingID]
	if ok {
		delete(m.byID, mappingID)
	}
	return e, ok
}

func (e *entry) auditDetail(clientAddr string) map[string]any {
	return map[string]any{
		"mappingId":  e.runtime.MappingID,
		"direction":  e.runtime.Direction,
		"listenHost": e.runtime.ListenHost,
		"listenPort": e.runtime.ListenPort,
		"targetHost": e.runtime.TargetHost,
		"targetPort": e.runtime.TargetPort,
		"clientAddr": clientAddr,
	}
}

func (e *entry) snapshot() runtime {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.runtime
	r.BytesIn = atomic.LoadInt64(&e.runtime.BytesIn)
	r.BytesOut = atomic.LoadInt64(&e.runtime.BytesOut)
	r.ActiveConns = atomic.LoadInt64(&e.runtime.ActiveConns)
	return r
}

func (e *entry) setListening(ok bool, listenPort int, errMsg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runtime.Listening = ok
	if listenPort > 0 {
		e.runtime.ListenPort = listenPort
	}
	e.runtime.LastError = errMsg
}
