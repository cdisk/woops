package app

import (
	"context"

	"github.com/ops-bastion/ops/go/internal/agent/plugins/monitor"
)

func init() {
	registerFeature("plugin.monitor", func(ctx *buildContext) {
		ctx.registerBackground(func(runCtx context.Context) {
			monitor.Start(runCtx, monitor.Config{
				MetricsWS:  ctx.cfg.MetricsWS,
				AssetID:    ctx.cfg.AssetID,
				AgentToken: ctx.cfg.AgentToken,
				Interval:   ctx.cfg.MetricsInterval,
				Enabled:    ctx.cfg.MetricsEnabled,
				ConfigPath: ctx.cfg.ConfigPath,
				Dialer:     ctx.services.Dialer,
			})
		})
	})
}
