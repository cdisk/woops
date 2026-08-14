package app

import "github.com/ops-bastion/ops/go/internal/gateway/sessions/shell"

func init() {
	registerFeature("session.shell", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("shell")
		shell.Register(ctx.server, shell.Deps{
			Serve:            ctx.server.ServeBridge,
			RecordingDir:     ctx.server.CreateRecordingDir,
			RecordingRelPath: ctx.server.RecordingRelPath,
		})
	})
}
