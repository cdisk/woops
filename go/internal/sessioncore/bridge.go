package sessioncore

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// AuditFields identifies a session operation without exposing protocol claims.
type AuditFields struct {
	OperationID string
	AssetID     string
	UserID      string
	Username    string
}

// BridgePrepared is the protocol-owned portion of a generic bridge.
type BridgePrepared struct {
	Params             any
	StartDetail        map[string]any
	BrowserToAgent     FrameHook
	AgentToBrowser     FrameHook
	BrowserToAgentSize FrameSizer
	AgentToBrowserSize FrameSizer
	Finish             func() (success bool, detail map[string]any)
	Flush              func()
	Interrupt          func() map[string]any
}

// BridgeSpec describes one protocol without teaching the bridge its claims.
type BridgeSpec struct {
	ExpectedTypes []string
	Protocol      string
	OperationType string
	Prepare       func(BaseClaims, json.RawMessage) (BridgePrepared, error)
}

// BridgeServeFunc is the narrow host callback used by protocol HTTP handlers.
type BridgeServeFunc func(BridgeSpec, http.ResponseWriter, *http.Request)

// FrameHook observes a frame after it has been forwarded. It must return
// quickly: the copy goroutine cannot read the next frame until it does.
type FrameHook func(messageType int, data []byte)

// FrameSizer returns the audited payload size for a forwarded frame. Nil means
// the complete WebSocket payload length. Datagram protocols use this to omit
// their routing header from traffic totals.
type FrameSizer func(messageType int, data []byte) int64

// CopyWSTee pipes frames one way. counted, when non-nil, accumulates the
// forwarded payload size for the operation's END detail; hook, when non-nil,
// observes each frame after it is forwarded so recording never sits between
// reading a frame and delivering it.
func CopyWSTee(dst, src *websocket.Conn, counted *int64, hook FrameHook, sizer FrameSizer) error {
	for {
		mt, data, err := src.ReadMessage()
		if err != nil {
			return err
		}
		// Application frames also prove the peer is alive (in addition to Pong).
		_ = src.SetReadDeadline(time.Now().Add(KeepAliveIdle))
		if err := dst.WriteMessage(mt, data); err != nil {
			return err
		}
		if counted != nil {
			size := int64(len(data))
			if sizer != nil {
				size = sizer(mt, data)
				if size < 0 {
					size = 0
				}
			}
			atomic.AddInt64(counted, size)
		}
		if hook != nil {
			hook(mt, data)
		}
	}
}
