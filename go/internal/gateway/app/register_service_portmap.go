package app

import (
	"github.com/ops-bastion/ops/go/internal/gateway/core"
	"github.com/ops-bastion/ops/go/internal/gateway/services/portmap"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func init() {
	registerFeature("service.portmap", func(ctx *buildContext) {
		s := ctx.server
		service := portmap.NewService(portmap.Deps{
			PostJSON:     s.PostJSON,
			GetJSON:      s.GetJSON,
			HasAgent:     s.HasAgent,
			SendAgent:    s.SendAgent,
			Arm:          s.ArmSession,
			Wait:         s.WaitSession,
			ServeBridge:  s.ServeBridge,
			VerifyTicket: s.VerifyTicket,
			UpgradeWS:    s.UpgradeWS,
			OpenSession: func(assetID, sessionID, ticket, protocol, host string, port int) error {
				return s.OpenAgentSession(assetID, sessionID, ticket, protocol,
					control.TunnelParams{TargetHost: host, TargetPort: port})
			},
			AuditStart:       s.OperationStart,
			AuditEnd:         s.OperationEnd,
			AuditStartClaims: s.AuditStart,
			AuditEndClaims:   s.AuditEnd,
		})

		s.AllowAgentSessionTypes("tcp", "udp")
		s.RegisterControlHandler("portmap_listen_status", func(agent core.ControlContext, env control.Envelope) error {
			payload, err := control.UnmarshalPayload[control.PortmapListenStatusPayload](env)
			if err != nil {
				return nil
			}
			service.HandleListenStatus(agent.AssetID, payload)
			return nil
		})
		s.RegisterAgentOnlineHook(service.RestoreForAsset)
		s.RegisterAgentOfflineHook(service.DropReverseForAsset)
		s.Register("/api/agent/portmap/open-connection", service.HandleAgentOpen)
		s.Register("/ws/opsctl/portmap-forward/tcp", service.HandleOpsctlForward("tcp"))
		s.Register("/ws/opsctl/portmap-forward/udp", service.HandleOpsctlForward("udp"))
		s.Register("/ws/opsctl/portmap-reverse-control", service.HandleOpsctlReverseControl)
		s.Register("/ws/opsctl/portmap-reverse-data", service.HandleOpsctlReverseData)
		s.Register("/internal/portmap/listening-ports", service.HandleListeningPorts)
		s.Register("/internal/portmap/list", service.HandleList)
		s.Register("/internal/portmap/open", service.HandleOpen)
		s.Register("/internal/portmap/close", service.HandleClose)
	})
}
