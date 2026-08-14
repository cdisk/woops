package desktop

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/gateway/core"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type Claims struct {
	sessioncore.BaseClaims
	TargetHost         string `json:"targetHost"`
	TargetPort         int    `json:"targetPort"`
	DesktopUsername    string `json:"desktopUsername"`
	DesktopPassword    string `json:"desktopPassword"`
	Hostname           string `json:"hostname"`
	DesktopColorDepth  int    `json:"desktopColorDepth,omitempty"`
	DesktopRdpQuality  string `json:"desktopRdpQuality,omitempty"`
}

type Params = control.TunnelParams

type Deps struct {
	VerifyTicket     func(string) (sessioncore.BaseClaims, json.RawMessage, error)
	Upgrade          func(http.ResponseWriter, *http.Request) (*websocket.Conn, error)
	Arm              func(string)
	Wait             func(string, time.Duration) (*websocket.Conn, error)
	OpenAgentSession func(string, string, string, string, any) error
	AuditStart       func(string, sessioncore.BaseClaims, map[string]any)
	AuditEnd         func(string, sessioncore.BaseClaims, bool, map[string]any)
	AttachFinalize   func(string, func() map[string]any)
	RecordingRoot    func() string
	RecordingDir     func(string) (string, error)
	RecordingRelPath func(string) string
	GuacdAddr        string
	GuacBridgeHost   string
}

func Register(r core.Router, d Deps) {
	if d.VerifyTicket == nil || d.Upgrade == nil {
		return
	}
	r.Register("/ws/desktop", NewHandler(d).ServeHTTP)
}
