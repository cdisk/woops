package audit

// Envelope phases, operation types and statuses. These strings must match
// com.ops.control.serverops.ServerOperationAuditService.
const (
	PhaseStart  = "START"
	PhaseAction = "ACTION"
	PhaseError  = "ERROR"
	PhaseEnd    = "END"

	TypeShell      = "SHELL"
	TypeFile       = "FILE"
	TypeExec       = "EXEC"
	TypeRDP        = "RDP"
	TypeVNC        = "VNC"
	TypePortmapTCP = "PORTMAP_TCP"
	TypePortmapUDP = "PORTMAP_UDP"

	StatusRunning     = "RUNNING"
	StatusCompleted   = "COMPLETED"
	StatusFailed      = "FAILED"
	StatusInterrupted = "INTERRUPTED"
)

// TypeForProtocol maps a session/ticket protocol to an operation type.
// Unknown protocols return "" so callers can skip auditing rather than
// inventing a type the console cannot render.
func TypeForProtocol(protocol string) string {
	switch protocol {
	case "shell", "shell_bash", "shell_powershell":
		return TypeShell
	case "filemanager", "filetransfer":
		return TypeFile
	case "exec":
		return TypeExec
	case "rdp":
		return TypeRDP
	case "vnc":
		return TypeVNC
	case "tcp":
		return TypePortmapTCP
	case "udp":
		return TypePortmapUDP
	default:
		return ""
	}
}
