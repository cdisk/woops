package app

import "github.com/ops-bastion/ops/go/internal/gateway/sessions/desktop"

func init() {
	registerFeature("session.desktop", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("rdp", "vnc")
		desktop.Register(ctx.server, desktop.Deps{
			VerifyTicket:     ctx.server.VerifyTicket,
			Upgrade:          ctx.server.UpgradeWS,
			Arm:              ctx.server.ArmSession,
			Wait:             ctx.server.WaitSession,
			OpenAgentSession: ctx.server.OpenAgentSession,
			AuditStart:       ctx.server.AuditStart,
			AuditEnd:         ctx.server.AuditEnd,
			AttachFinalize:   ctx.server.AttachAuditFinalize,
			RecordingRoot:    ctx.server.RecordingRoot,
			RecordingDir:     ctx.server.CreateRecordingDir,
			RecordingRelPath: ctx.server.RecordingRelPath,
			GuacdAddr:        ctx.server.GuacdAddr(),
			GuacBridgeHost:   ctx.server.GuacBridgeHost(),
		})
	})
}
