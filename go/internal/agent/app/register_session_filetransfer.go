package app

import (
	"log"

	"github.com/ops-bastion/ops/go/internal/agent/core"
	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
	"github.com/ops-bastion/ops/go/internal/agent/sessions/filetransfer"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func init() {
	registerFeature("session.filetransfer", func(ctx *buildContext) {
		ctx.registerSession("filetransfer", "FILE", filetransfer.Run)
		ctx.registerControl(func(_ core.ControlSender, reg *sessionreg.ControlRegistry) {
			reg.Register("abort_transfer", func(env control.Envelope) error {
				payload, err := control.UnmarshalPayload[control.AbortTransferPayload](env)
				if err != nil {
					return nil
				}
				if err := filetransfer.AbortTransfer(payload.Path, payload.TransferID); err != nil {
					log.Printf("abort_transfer err=%v", err)
				} else {
					log.Printf("abort_transfer ok transfer=%s path=%s", payload.TransferID, payload.Path)
				}
				return nil
			})
		})
	})
}
