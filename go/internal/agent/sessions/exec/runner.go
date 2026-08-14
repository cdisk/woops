package exec

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

func Run(ws *websocket.Conn, _ json.RawMessage) error {
	ServeJSON(ws)
	return nil
}
