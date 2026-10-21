package sessioncore

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type pendingResult struct {
	ws  *websocket.Conn
	err error
}

type pendingEntry struct {
	ch        chan pendingResult
	createdAt time.Time
}

// PendingSessions coordinates browser-side Wait with agent-side Deliver for a
// single session data WebSocket handoff.
type PendingSessions struct {
	mu   sync.Mutex
	wait map[string]*pendingEntry
}

func NewPendingSessions() *PendingSessions {
	return &PendingSessions{wait: map[string]*pendingEntry{}}
}

func (p *PendingSessions) Arm(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.wait[sessionID]; ok {
		return
	}
	p.wait[sessionID] = &pendingEntry{ch: make(chan pendingResult, 1), createdAt: time.Now()}
}

func (p *PendingSessions) Wait(sessionID string, timeout time.Duration) (*websocket.Conn, error) {
	p.mu.Lock()
	entry, ok := p.wait[sessionID]
	if !ok {
		entry = &pendingEntry{ch: make(chan pendingResult, 1), createdAt: time.Now()}
		p.wait[sessionID] = entry
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.wait, sessionID)
		p.mu.Unlock()
	}()

	select {
	case res := <-entry.ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.ws, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting agent session %s", sessionID)
	}
}

// Cancel unblocks Wait immediately. Used when open_session cannot be sent.
func (p *PendingSessions) Cancel(sessionID string) bool {
	return p.finish(sessionID, pendingResult{err: fmt.Errorf("agent session cancelled")})
}

func (p *PendingSessions) Deliver(sessionID string, ws *websocket.Conn) bool {
	return p.finish(sessionID, pendingResult{ws: ws})
}

func (p *PendingSessions) finish(sessionID string, res pendingResult) bool {
	p.mu.Lock()
	entry, ok := p.wait[sessionID]
	p.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case entry.ch <- res:
		return true
	default:
		return false
	}
}
