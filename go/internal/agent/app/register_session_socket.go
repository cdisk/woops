package app

import "github.com/ops-bastion/ops/go/internal/agent/sessions/socket"

func init() {
	registerFeature("session.socket", func(ctx *buildContext) {
		ctx.registerSession("rdp", "RDP", socket.RunTCP)
		ctx.registerSession("vnc", "VNC", socket.RunTCP)
		ctx.registerSession("tcp", "TCP", socket.RunTCP)
		ctx.registerSession("udp", "UDP", socket.RunUDP)
	})
}
