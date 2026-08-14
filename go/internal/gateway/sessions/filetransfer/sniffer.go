package filetransfer

import (
	"sync"

	"github.com/gorilla/websocket"
	filetransferproto "github.com/ops-bastion/ops/go/internal/protocol/filetransfer"
)

// Opts identifies the transfer for audit detail (no payloads).
type Opts struct {
	TransferID string
	Direction  string
	Path       string
}

// Sniffer observes filetransfer control + binary headers for audit.
// Payloads and checksums are never recorded.
type Sniffer struct {
	emit func(eventType string, detail map[string]any)
	opts Opts

	mu     sync.Mutex
	bytes  int64
	chunks int
	offset int64
	have   bool
}

// NewSniffer builds a transfer sniffer.
func NewSniffer(emit func(eventType string, detail map[string]any), opts Opts) *Sniffer {
	return &Sniffer{emit: emit, opts: opts}
}

// Observe inspects one forwarded frame.
func (s *Sniffer) Observe(mt int, data []byte, browserToAgent bool) {
	if s == nil || s.emit == nil {
		return
	}
	if mt == websocket.BinaryMessage {
		offset, length, ok := filetransferproto.PeekHeader(data)
		if !ok || length <= 0 {
			return
		}
		s.mu.Lock()
		if !s.have {
			s.offset = offset
			s.have = true
		}
		s.bytes += int64(length)
		s.chunks++
		s.mu.Unlock()
		return
	}
	if mt != websocket.TextMessage {
		return
	}
	ctrl, err := filetransferproto.DecodeControl(data)
	if err != nil {
		return
	}
	switch ctrl.Type {
	case filetransferproto.CtrlStatus:
		detail := map[string]any{
			"transferId": s.opts.TransferID,
			"direction":  s.opts.Direction,
			"path":       s.opts.Path,
			"offset":     ctrl.Offset,
			"size":       ctrl.Size,
			"resumed":    ctrl.Resumed,
		}
		s.emit("TRANSFER_STATUS", detail)
	case filetransferproto.CtrlCommitted:
		s.Flush()
		detail := map[string]any{
			"transferId": s.opts.TransferID,
			"direction":  s.opts.Direction,
			"path":       s.opts.Path,
			"size":       ctrl.Size,
			"result":     "committed",
		}
		s.emit("TRANSFER_COMMIT", detail)
	case filetransferproto.CtrlAborted:
		s.Flush()
		detail := map[string]any{
			"transferId": s.opts.TransferID,
			"direction":  s.opts.Direction,
			"path":       s.opts.Path,
			"result":     "aborted",
		}
		s.emit("TRANSFER_ABORT", detail)
	case filetransferproto.CtrlError:
		detail := map[string]any{
			"transferId": s.opts.TransferID,
			"direction":  s.opts.Direction,
			"path":       s.opts.Path,
			"message":    ctrl.Message,
		}
		s.emit("TRANSFER_ERROR", detail)
	case filetransferproto.CtrlAbort:
		if browserToAgent {
			detail := map[string]any{
				"transferId": s.opts.TransferID,
				"direction":  s.opts.Direction,
				"path":       s.opts.Path,
				"result":     "abort_requested",
			}
			s.emit("TRANSFER_ABORT", detail)
		}
	}
}

// Flush emits a READ/WRITE aggregate for buffered binary chunks.
func (s *Sniffer) Flush() {
	if s == nil || s.emit == nil {
		return
	}
	s.mu.Lock()
	if !s.have || s.chunks == 0 {
		s.mu.Unlock()
		return
	}
	detail := map[string]any{
		"transferId": s.opts.TransferID,
		"direction":  s.opts.Direction,
		"path":       s.opts.Path,
		"offset":     s.offset,
		"bytes":      s.bytes,
		"chunks":     s.chunks,
	}
	event := "WRITE"
	if s.opts.Direction == filetransferproto.DirectionDownload {
		event = "READ"
	}
	s.bytes, s.chunks, s.offset, s.have = 0, 0, 0, false
	s.mu.Unlock()
	s.emit(event, detail)
}
