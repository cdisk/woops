package filetransfer

import (
	"encoding/json"
	"net/http"

	"github.com/ops-bastion/ops/go/internal/gateway/core"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type Claims struct {
	sessioncore.BaseClaims
	TransferID  string `json:"transferId"`
	Direction   string `json:"direction"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Fingerprint string `json:"fingerprint"`
	Abort       bool   `json:"abort"`
}

type Params = control.FileTransferParams

type Deps struct {
	Serve      sessioncore.BridgeServeFunc
	AuditEvent func(string, sessioncore.BaseClaims, string, map[string]any)
}

func Register(r core.Router, d Deps) {
	if d.Serve == nil {
		return
	}
	r.Register("/ws/file-transfer", Handler(d))
}

func Handler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Serve(sessioncore.BridgeSpec{
			ExpectedTypes: []string{"filetransfer"}, Protocol: "filetransfer",
			OperationType: auditstore.TypeFile,
			Prepare: func(base sessioncore.BaseClaims, raw json.RawMessage) (sessioncore.BridgePrepared, error) {
				var claims Claims
				if err := json.Unmarshal(raw, &claims); err != nil {
					return sessioncore.BridgePrepared{}, err
				}
				params := Params{TransferID: claims.TransferID, Direction: claims.Direction, Path: claims.Path,
					Size: claims.Size, Fingerprint: claims.Fingerprint, Abort: claims.Abort}
				detail := map[string]any{"transferId": claims.TransferID, "direction": claims.Direction, "path": claims.Path}
				if claims.Abort {
					detail["abort"] = true
				}
				var sniffer *Sniffer
				if d.AuditEvent != nil {
					sniffer = NewSniffer(func(event string, detail map[string]any) {
						d.AuditEvent(auditstore.TypeFile, base, event, detail)
					}, Opts{TransferID: claims.TransferID, Direction: claims.Direction, Path: claims.Path})
				}
				return sessioncore.BridgePrepared{
					Params: params, StartDetail: detail,
					BrowserToAgent: func(mt int, data []byte) { sniffer.Observe(mt, data, true) },
					AgentToBrowser: func(mt int, data []byte) { sniffer.Observe(mt, data, false) },
					Flush:          sniffer.Flush,
				}, nil
			},
		}, w, r)
	}
}
