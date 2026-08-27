package app

import (
	"context"

	"github.com/ops-bastion/ops/go/internal/agent/plugins/proxy"
)

func init() {
	registerFeature("plugin.proxy", func(ctx *buildContext) {
		ctx.registerPreAuthBackground(func(runCtx context.Context) {
			proxy.TryStart(runCtx, proxy.Deps{
				Server:        ctx.cfg.Gateway,
				UpstreamProxy: ctx.cfg.GatewayProxy,
				Raw:           ctx.cfg.ProxyRaw,
				BridgeRaw:     ctx.cfg.ProxyBridgeRaw,
				ConfigPath:    ctx.cfg.ConfigPath,
			})
		})
	})
}
