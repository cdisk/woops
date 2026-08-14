package app

import "github.com/ops-bastion/ops/go/internal/gateway/sessions/filetransfer"

func init() {
	registerFeature("session.filetransfer", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("filetransfer")
		filetransfer.Register(ctx.server, filetransfer.Deps{
			Serve:      ctx.server.ServeBridge,
			AuditEvent: ctx.server.AuditEvent,
		})
	})
}
