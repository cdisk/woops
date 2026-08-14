package control

import "encoding/json"

const ProtocolVersion = 1

type Envelope struct {
	Type      string          `json:"type"`
	Version   int             `json:"version"`
	RequestID string          `json:"requestId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// OpenSessionPayload is the control-plane open_session body (nested Params only).
type OpenSessionPayload struct {
	SessionID string          `json:"sessionId"`
	Protocol  string          `json:"protocol"` // shell | filemanager | filetransfer | exec | rdp | vnc | tcp | udp
	Ticket    string          `json:"ticket"`
	Params    json.RawMessage `json:"params,omitempty"`
}

// ShellParams are protocol-specific open_session params for shell.
type ShellParams struct {
	ShellKind string `json:"shellKind,omitempty"` // bash | powershell
	Cols      int    `json:"cols,omitempty"`      // optional initial PTY width
	Rows      int    `json:"rows,omitempty"`      // optional initial PTY height
}

// FileTransferParams are protocol-specific open_session params for filetransfer.
type FileTransferParams struct {
	TransferID  string `json:"transferId,omitempty"`
	Direction   string `json:"direction,omitempty"` // upload | download
	Path        string `json:"path,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Abort       bool   `json:"abort,omitempty"`
}

// TunnelParams are protocol-specific open_session params for rdp/vnc/tcp/udp.
type TunnelParams struct {
	TargetHost string `json:"targetHost,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
}

// AbortTransferPayload asks the Agent to discard an upload's temp files
// without opening a data session (cancel while paused).
type AbortTransferPayload struct {
	TransferID string `json:"transferId"`
	Path       string `json:"path"`
}

// PortmapListenPayload asks the Agent to bind a reverse (asset→gateway) mapping.
type PortmapListenPayload struct {
	MappingID  string `json:"mappingId"`
	Protocol   string `json:"protocol"` // tcp | udp
	ListenHost string `json:"listenHost"`
	ListenPort int    `json:"listenPort"`
	TargetHost string `json:"targetHost,omitempty"` // informational; Agent must not dial for reverse
	TargetPort int    `json:"targetPort,omitempty"`
}

// PortmapUnlistenPayload stops a reverse mapping listener on the Agent.
type PortmapUnlistenPayload struct {
	MappingID string `json:"mappingId"`
}

// PortmapListenStatusPayload is Agent → Gateway after bind attempt.
type PortmapListenStatusPayload struct {
	MappingID  string `json:"mappingId"`
	OK         bool   `json:"ok"`
	ListenHost string `json:"listenHost,omitempty"`
	ListenPort int    `json:"listenPort,omitempty"`
	Error      string `json:"error,omitempty"`
}

type AckPayload struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type NetInfoPayload struct {
	PrivateIP string `json:"privateIp,omitempty"` // CSV of private IPv4 addresses
}

func Marshal(typ string, requestID string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{
		Type:      typ,
		Version:   ProtocolVersion,
		RequestID: requestID,
		Payload:   raw,
	})
}

func UnmarshalPayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	err := json.Unmarshal(env.Payload, &out)
	return out, err
}
