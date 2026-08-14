// Package filetransfer defines the binary file-transfer session framing shared by
// Agent, Gateway sniffers, Console and opsctl. Payload bytes stay raw; only
// control messages are JSON text frames.
package filetransfer

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	Magic   = "OPST"
	Version = byte(1)

	// MaxChunk is the maximum binary payload size per frame (1 MiB).
	MaxChunk = 1024 * 1024

	// HeaderLen = magic(4) + ver(1) + reserved(1) + offset(8) + length(4) + sha256(32)
	HeaderLen = 50

	CtrlHello     = "hello"
	CtrlStatus    = "status"
	CtrlAck       = "ack"
	CtrlCommit    = "commit"
	CtrlCommitted = "committed"
	CtrlAbort     = "abort"
	CtrlAborted   = "aborted"
	CtrlError     = "error"
	CtrlRequest   = "request" // download: client asks for chunks from offset

	DirectionUpload   = "upload"
	DirectionDownload = "download"
)

var (
	ErrBadMagic   = errors.New("xfer: bad magic")
	ErrBadVersion = errors.New("xfer: unsupported version")
	ErrBadLength  = errors.New("xfer: bad length")
	ErrChecksum   = errors.New("xfer: chunk checksum mismatch")
)

// Control is a JSON text frame on the transfer WebSocket.
type Control struct {
	Type        string `json:"type"`
	TransferID  string `json:"transferId,omitempty"`
	Direction   string `json:"direction,omitempty"`
	Path        string `json:"path,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Offset      int64  `json:"offset,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	OK          bool   `json:"ok,omitempty"`
	Resumed     bool   `json:"resumed,omitempty"`
	Abort       bool   `json:"abort,omitempty"`
	Message     string `json:"message,omitempty"`
	Mtime       int64  `json:"mtime,omitempty"`
}

func EncodeControl(c Control) ([]byte, error) {
	return json.Marshal(c)
}

func DecodeControl(data []byte) (Control, error) {
	var c Control
	if err := json.Unmarshal(data, &c); err != nil {
		return Control{}, err
	}
	return c, nil
}

// EncodeChunk builds a binary frame for one chunk.
func EncodeChunk(offset int64, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrBadLength
	}
	if len(data) > MaxChunk {
		return nil, fmt.Errorf("xfer: chunk too large (%d > %d)", len(data), MaxChunk)
	}
	sum := sha256.Sum256(data)
	out := make([]byte, HeaderLen+len(data))
	copy(out[0:4], Magic)
	out[4] = Version
	out[5] = 0
	binary.BigEndian.PutUint64(out[6:14], uint64(offset))
	binary.BigEndian.PutUint32(out[14:18], uint32(len(data)))
	copy(out[18:50], sum[:])
	copy(out[50:], data)
	return out, nil
}

// DecodeChunk parses and verifies a binary frame. Returns offset and payload.
func DecodeChunk(frame []byte) (offset int64, data []byte, err error) {
	if len(frame) < HeaderLen {
		return 0, nil, ErrBadLength
	}
	if string(frame[0:4]) != Magic {
		return 0, nil, ErrBadMagic
	}
	if frame[4] != Version {
		return 0, nil, ErrBadVersion
	}
	offset = int64(binary.BigEndian.Uint64(frame[6:14]))
	n := int(binary.BigEndian.Uint32(frame[14:18]))
	if n < 0 || n > MaxChunk || HeaderLen+n != len(frame) {
		return 0, nil, ErrBadLength
	}
	want := frame[18:50]
	data = frame[50 : 50+n]
	sum := sha256.Sum256(data)
	for i := 0; i < 32; i++ {
		if sum[i] != want[i] {
			return 0, nil, ErrChecksum
		}
	}
	return offset, data, nil
}

// PeekHeader extracts offset/length from a binary frame without copying payload.
// Used by Gateway audit sniffers; does not verify checksum.
func PeekHeader(frame []byte) (offset int64, length int, ok bool) {
	if len(frame) < HeaderLen {
		return 0, 0, false
	}
	if string(frame[0:4]) != Magic || frame[4] != Version {
		return 0, 0, false
	}
	offset = int64(binary.BigEndian.Uint64(frame[6:14]))
	length = int(binary.BigEndian.Uint32(frame[14:18]))
	if length < 0 || length > MaxChunk || HeaderLen+length > len(frame) {
		return 0, 0, false
	}
	return offset, length, true
}
