package app

import "github.com/ops-bastion/ops/go/internal/gateway/services/agentinstall"

func init() {
	registerFeature("service.agentinstall", func(ctx *buildContext) {
		agentinstall.Register(ctx.server, agentinstall.Deps{
			GetJSON:        ctx.server.GetJSON,
			PublicHTTPBase: ctx.cfg.PublicHTTPBase,
			AgentBinDir:    ctx.cfg.AgentBinDir,
			TLSSpkiSHA256:  ctx.cfg.TLSSpkiSHA256,
		})
		// Same bin dir as agents; public download (no install code).
		agentinstall.RegisterWoopsctlBin(ctx.server, ctx.cfg.AgentBinDir)
	})
}
