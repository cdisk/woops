package filemanager

import (
	"encoding/json"
	"strings"
	"sync"
)

// Sniffer turns filemanager JSON-RPC calls into ACTION events.
// Content transfer is audited by filetransfer.Sniffer on /ws/file-transfer.
//
// Best effort by design: unparseable frames and unknown methods are ignored so
// a protocol change can never break the live session.
type Sniffer struct {
	emit func(eventType string, detail map[string]any)

	mu sync.Mutex
}

type fileRPC struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// NewSniffer builds a sniffer that emits via the given callback.
func NewSniffer(emit func(eventType string, detail map[string]any)) *Sniffer {
	return &Sniffer{emit: emit}
}

// Observe inspects one browser → agent text frame.
func (s *Sniffer) Observe(frame []byte) {
	if s == nil || s.emit == nil {
		return
	}
	var rpc fileRPC
	if err := json.Unmarshal(frame, &rpc); err != nil {
		return
	}
	method := strings.ToLower(strings.TrimSpace(rpc.Method))
	path, _ := rpc.Params["path"].(string)
	switch method {
	case "list", "stat", "mkdir", "remove", "rename", "roots":
		detail := map[string]any{"path": path}
		if to, ok := rpc.Params["to"].(string); ok && to != "" {
			detail["to"] = to
		}
		s.emit(strings.ToUpper(method), detail)
	}
}

// Flush is a no-op for filemanager (no chunk aggregates).
func (s *Sniffer) Flush() {}
