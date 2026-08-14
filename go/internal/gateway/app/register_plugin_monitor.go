package app

import "github.com/ops-bastion/ops/go/internal/gateway/plugins/monitor"

func init() {
	registerFeature("plugin.monitor", func(ctx *buildContext) {
		ctx.server.Register("/ws/agent/metrics", monitor.Handle(ctx.server.UpgradeWS, ctx.server))
	})
}
