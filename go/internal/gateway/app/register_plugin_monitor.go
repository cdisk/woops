package app

import (
	"github.com/ops-bastion/ops/go/internal/gateway/core"
	"github.com/ops-bastion/ops/go/internal/gateway/plugins/monitor"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
)

func init() {
	registerFeature("plugin.monitor", func(ctx *buildContext) {
		ingestor := monitor.NewIngestor(ctx.server)
		ctx.server.Register("/ws/agent/metrics", monitor.Handle(ctx.server.UpgradeWS, ctx.server, ingestor))
		ctx.server.RegisterControlHandler("metrics_points", func(agent core.ControlContext, env control.Envelope) error {
			payload, err := control.UnmarshalPayload[monitorproto.PointsPayload](env)
			if err != nil {
				return nil
			}
			ingestor.Enqueue(agent.AssetID, agent.AgentToken, payload)
			return nil
		})
	})
}
