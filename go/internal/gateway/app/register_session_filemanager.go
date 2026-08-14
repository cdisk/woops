package app

import "github.com/ops-bastion/ops/go/internal/gateway/sessions/filemanager"

func init() {
	registerFeature("session.filemanager", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("filemanager")
		filemanager.Register(ctx.server, filemanager.Deps{
			Serve:      ctx.server.ServeBridge,
			AuditEvent: ctx.server.AuditEvent,
		})
	})
}
