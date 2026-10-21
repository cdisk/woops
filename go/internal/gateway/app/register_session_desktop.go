package app

import (
	"time"

	"github.com/ops-bastion/ops/go/internal/gateway/sessions/desktop"
)

func init() {
	registerFeature("session.desktop", func(ctx *buildContext) {
		ctx.server.AllowAgentSessionTypes("rdp", "vnc")
		desktop.Register(ctx.server, desktop.Deps{
			VerifyTicket:     ctx.server.VerifyTicket,
			Upgrade:          ctx.server.UpgradeWS,
			Arm:              ctx.server.ArmSession,
			Wait:             ctx.server.WaitSession,
			OpenAgentSession: ctx.server.OpenAgentSession,
			Cancel:           ctx.server.CancelSession,
			KeepAlive:        func() time.Duration { return ctx.server.PingFor("desktop") },
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
