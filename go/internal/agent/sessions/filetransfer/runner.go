package filetransfer

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func Run(ws *websocket.Conn, params json.RawMessage) error {
	var p control.FileTransferParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return err
		}
	}
	Serve(ws, SessionParams{
		TransferID:  p.TransferID,
		Direction:   p.Direction,
		Path:        p.Path,
		Size:        p.Size,
		Fingerprint: p.Fingerprint,
		Abort:       p.Abort,
	})
	return nil
}
