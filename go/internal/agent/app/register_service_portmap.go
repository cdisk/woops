package app

import (
	"fmt"
	"log"

	"github.com/ops-bastion/ops/go/internal/agent/core"
	"github.com/ops-bastion/ops/go/internal/agent/services/portmap"
	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func init() {
	registerFeature("service.portmap", func(ctx *buildContext) {
		manager := portmap.NewManager(portmap.Deps{
			OpenConnection: func(mappingID, clientAddr string) (string, string, error) {
				var response struct {
					SessionID string `json:"sessionId"`
					Ticket    string `json:"ticket"`
				}
				err := ctx.services.PostJSON("/api/agent/portmap/open-connection", map[string]string{
					"mappingId": mappingID, "clientAddr": clientAddr,
				}, &response)
				if err != nil {
					return "", "", err
				}
				if response.SessionID == "" || response.Ticket == "" {
					return "", "", fmt.Errorf("open-connection incomplete response")
				}
				return response.SessionID, response.Ticket, nil
			},
			DialSessionWS: ctx.services.DialSessionWS,
		})
		ctx.registerControl(func(send core.ControlSender, reg *sessionreg.ControlRegistry) {
			reg.Register("portmap_listen", func(env control.Envelope) error {
				payload, err := control.UnmarshalPayload[control.PortmapListenPayload](env)
				if err != nil {
					return nil
				}
				go manager.Listen(payload, func(status control.PortmapListenStatusPayload) {
					if err := send("portmap_listen_status", status.MappingID, status); err != nil {
						log.Printf("portmap status send err=%v", err)
					}
				})
				return nil
			})
			reg.Register("portmap_unlisten", func(env control.Envelope) error {
				payload, err := control.UnmarshalPayload[control.PortmapUnlistenPayload](env)
				if err == nil {
					manager.Unlisten(payload.MappingID)
				}
				return nil
			})
		})
		ctx.registerDisconnect(manager.CloseAll)
	})
}
