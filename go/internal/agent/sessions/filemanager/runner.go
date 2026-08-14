package filemanager

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

func Run(ws *websocket.Conn, _ json.RawMessage) error {
	ServeJSONRPC(ws)
	return nil
}
