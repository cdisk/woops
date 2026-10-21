package app

import (
	"github.com/ops-bastion/ops/go/internal/agent/core"
	"github.com/ops-bastion/ops/go/internal/agent/plugins/monitor"
	"github.com/ops-bastion/ops/go/internal/agent/sessionreg"
)

func init() {
	registerFeature("plugin.monitor", func(ctx *buildContext) {
		if !ctx.cfg.MetricsEnabled {
			return
		}
		reporter := monitor.NewReporter(ctx.cfg.MetricsInterval)
		ctx.registerControl(func(send core.ControlSender, _ *sessionreg.ControlRegistry) {
			reporter.OnConnect(monitor.Sender(send))
		})
		ctx.registerDisconnect(reporter.OnDisconnect)
	})
}
