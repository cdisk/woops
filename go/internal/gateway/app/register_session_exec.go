package app

import execsession "github.com/ops-bastion/ops/go/internal/gateway/sessions/exec"

func init() {
	registerFeature("session.exec", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("exec")
		execsession.Register(ctx.server, execsession.Deps{
			Serve:      ctx.server.ServeBridge,
			AuditEvent: ctx.server.AuditEvent,
		})
	})
}
