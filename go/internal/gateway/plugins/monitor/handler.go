package monitor

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
	"github.com/ops-bastion/ops/go/internal/wsutil"
)

// Poster posts JSON to control-api.
type Poster interface {
	PostJSON(path string, body any, out any) error
}

// Handle returns an HTTP handler for the dedicated agent metrics WebSocket.
func Handle(upgrade func(http.ResponseWriter, *http.Request) (*websocket.Conn, error), post Poster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assetID := r.Header.Get("X-Asset-Id")
		token := r.Header.Get("X-Agent-Token")
		if assetID == "" || token == "" {
			http.Error(w, "missing agent credentials", http.StatusUnauthorized)
			return
		}
		var auth map[string]any
		if err := post.PostJSON("/api/internal/agent/auth", map[string]any{
			"assetId": assetID, "agentToken": token,
		}, &auth); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		ws, err := upgrade(w, r)
		if err != nil {
			return
		}
		defer ws.Close()
		log.Printf("metrics ws online: %s", assetID)

		wsutil.EnableReadDeadline(ws, wsutil.MetricsIdle)

		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				log.Printf("metrics ws offline: %s", assetID)
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(wsutil.MetricsIdle))
			var env monitorproto.Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				continue
			}
			switch env.Type {
			case "points":
				payload, err := monitorproto.UnmarshalPayload[monitorproto.PointsPayload](env)
				if err != nil {
					continue
				}
				points := make([]map[string]any, 0, len(payload.Points))
				for _, p := range payload.Points {
					points = append(points, map[string]any{
						"itemId":   p.ItemID,
						"instance": p.Instance,
						"value":    p.Value,
					})
				}
				body := map[string]any{
					"assetId":     assetID,
					"agentToken":  token,
					"collectedAt": payload.CollectedAt,
					"points":      points,
				}
				if err := post.PostJSON("/api/internal/metrics/ingest", body, nil); err != nil {
					log.Printf("metrics ingest failed asset=%s: %v", assetID, err)
					ack, _ := monitorproto.Marshal("ack", env.RequestID, monitorproto.AckPayload{OK: false, Message: err.Error()})
					_ = ws.WriteMessage(websocket.TextMessage, ack)
					continue
				}
				ack, _ := monitorproto.Marshal("ack", env.RequestID, monitorproto.AckPayload{OK: true})
				_ = ws.WriteMessage(websocket.TextMessage, ack)
			default:
				// ignore unknown
			}
		}
	}
}
