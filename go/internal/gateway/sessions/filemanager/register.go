package filemanager

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/gateway/core"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type Claims struct{ sessioncore.BaseClaims }
type Params struct{}

type Deps struct {
	Serve      sessioncore.BridgeServeFunc
	AuditEvent func(string, sessioncore.BaseClaims, string, map[string]any)
}

func Register(r core.Router, d Deps) {
	if d.Serve == nil {
		return
	}
	r.Register("/ws/file-manager", Handler(d))
}

func Handler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Serve(sessioncore.BridgeSpec{
			ExpectedTypes: []string{"filemanager"}, Protocol: "filemanager",
			OperationType: auditstore.TypeFile,
			Prepare: func(base sessioncore.BaseClaims, raw json.RawMessage) (sessioncore.BridgePrepared, error) {
				var claims Claims
				if err := json.Unmarshal(raw, &claims); err != nil {
					return sessioncore.BridgePrepared{}, err
				}
				var sniffer *Sniffer
				if d.AuditEvent != nil {
					sniffer = NewSniffer(func(event string, detail map[string]any) {
						d.AuditEvent(auditstore.TypeFile, base, event, detail)
					})
				}
				return sessioncore.BridgePrepared{
					BrowserToAgent: func(mt int, data []byte) {
						if mt == websocket.TextMessage {
							sniffer.Observe(data)
						}
					},
					Flush: sniffer.Flush,
				}, nil
			},
		}, w, r)
	}
}
