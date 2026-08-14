package socket

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func RunTCP(ws *websocket.Conn, params json.RawMessage) error {
	p, err := parseParams(params)
	if err != nil {
		return err
	}
	return runTCP(ws, p.TargetHost, p.TargetPort)
}

func RunUDP(ws *websocket.Conn, params json.RawMessage) error {
	p, err := parseParams(params)
	if err != nil {
		return err
	}
	return runUDP(ws, p.TargetHost, p.TargetPort)
}

func parseParams(params json.RawMessage) (control.TunnelParams, error) {
	var p control.TunnelParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return p, err
		}
	}
	return p, nil
}
