package app

import "github.com/ops-bastion/ops/go/internal/agent/sessions/filemanager"

func init() {
	registerFeature("session.filemanager", func(ctx *buildContext) {
		ctx.registerSession("filemanager", "FILE", filemanager.Run)
	})
}
