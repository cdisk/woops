package shell

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/gateway/core"
	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

type Claims struct {
	sessioncore.BaseClaims
	ShellKind string `json:"shellKind"`
}

type Params = control.ShellParams

type Deps struct {
	Serve            sessioncore.BridgeServeFunc
	RecordingDir     func(operationID string) (string, error)
	RecordingRelPath func(path string) string
}

func Register(r core.Router, d Deps) {
	if d.Serve == nil {
		return
	}
	r.Register("/ws/shell", Handler(d))
}

func Handler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Serve(sessioncore.BridgeSpec{
			ExpectedTypes: []string{"shell"},
			Protocol:      "shell",
			OperationType: auditstore.TypeShell,
			Prepare: func(base sessioncore.BaseClaims, raw json.RawMessage) (sessioncore.BridgePrepared, error) {
				var claims Claims
				if err := json.Unmarshal(raw, &claims); err != nil {
					return sessioncore.BridgePrepared{}, err
				}
				detail := map[string]any{}
				if claims.ShellKind != "" {
					detail["shellKind"] = claims.ShellKind
				}
				prepared := sessioncore.BridgePrepared{
					Params: Params{ShellKind: claims.ShellKind}, StartDetail: detail,
				}
				rec, path := startRecording(d, base.SessionID, CastTitle(claims.ShellKind, base.Username, base.AssetID))
				if rec == nil {
					return prepared, nil
				}
				prepared.AgentToBrowser = func(_ int, data []byte) { rec.WriteOutput(data) }
				prepared.BrowserToAgent = func(mt int, data []byte) {
					if mt == websocket.TextMessage {
						if cols, rows, ok := ParseResize(data); ok {
							rec.WriteResize(cols, rows)
							return
						}
					}
					rec.WriteInput(data)
				}
				finalize := func() map[string]any {
					sum, size, err := rec.Close()
					if err != nil {
						log.Printf("ops-audit cast close session=%s: %v", base.SessionID, err)
					}
					if path == "" || size <= 0 {
						return nil
					}
					return map[string]any{"recordingPath": path, "format": "asciinema", "sha256": sum, "size": size}
				}
				prepared.Interrupt = finalize
				prepared.Finish = func() (bool, map[string]any) { return true, finalize() }
				return prepared, nil
			},
		}, w, r)
	}
}
