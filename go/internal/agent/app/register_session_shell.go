package app

import "github.com/ops-bastion/ops/go/internal/agent/sessions/shell"

func init() {
	registerFeature("session.shell", func(ctx *buildContext) {
		ctx.registerSession("shell", "SHELL", shell.Run)
	})
}
