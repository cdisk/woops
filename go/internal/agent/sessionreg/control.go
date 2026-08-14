package sessionreg

import (
	"strings"
	"sync"

	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

// ControlHandler handles one control-plane envelope (open_session, abort_transfer, …).
type ControlHandler func(env control.Envelope) error

// ControlRegistry maps control message types to handlers.
type ControlRegistry struct {
	mu   sync.RWMutex
	byID map[string]ControlHandler
}

func NewControl() *ControlRegistry {
	return &ControlRegistry{byID: make(map[string]ControlHandler)}
}

func (r *ControlRegistry) Register(typ string, h ControlHandler) {
	key := strings.TrimSpace(typ)
	if key == "" || h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[key] = h
}

// Handle runs the registered handler for env.Type.
// Returns false if no handler is registered (caller may ignore unknown types).
func (r *ControlRegistry) Handle(env control.Envelope) (bool, error) {
	r.mu.RLock()
	h := r.byID[env.Type]
	r.mu.RUnlock()
	if h == nil {
		return false, nil
	}
	return true, h(env)
}
