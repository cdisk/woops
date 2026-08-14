package sessionreg

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// Runner handles one data-plane session after the Agent dials the session WSS.
type Runner func(ws *websocket.Conn, params json.RawMessage) error

// Registry maps open_session protocol names to runners.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]entry
}

type entry struct {
	run           Runner
	operationType string
}

func New() *Registry {
	return &Registry{byID: make(map[string]entry)}
}

func (r *Registry) Register(protocol, operationType string, run Runner) {
	key := strings.ToLower(strings.TrimSpace(protocol))
	if key == "" || run == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[key] = entry{run: run, operationType: strings.TrimSpace(operationType)}
}

func (r *Registry) Run(protocol string, ws *websocket.Conn, params json.RawMessage) error {
	key := strings.ToLower(strings.TrimSpace(protocol))
	r.mu.RLock()
	entry := r.byID[key]
	r.mu.RUnlock()
	if entry.run == nil {
		msg := fmt.Sprintf("unsupported protocol: %s", protocol)
		_ = ws.WriteMessage(websocket.TextMessage, []byte("ERROR "+msg))
		return fmt.Errorf("%s", msg)
	}
	return entry.run(ws, params)
}

func (r *Registry) OperationType(protocol string) string {
	key := strings.ToLower(strings.TrimSpace(protocol))
	r.mu.RLock()
	operationType := r.byID[key].operationType
	r.mu.RUnlock()
	if operationType != "" {
		return operationType
	}
	if key == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(key)
}
