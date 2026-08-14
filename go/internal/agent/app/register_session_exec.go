package app

import execsession "github.com/ops-bastion/ops/go/internal/agent/sessions/exec"

func init() {
	registerFeature("session.exec", func(ctx *buildContext) {
		ctx.registerSession("exec", "EXEC", execsession.Run)
	})
}
