package filetransfer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	filetransferproto "github.com/ops-bastion/ops/go/internal/protocol/filetransfer"
)

// SessionParams come from the open_session / ticket claims.
type SessionParams struct {
	TransferID  string
	Direction   string
	Path        string
	Size        int64
	Fingerprint string
	Abort       bool
}

// Serve runs one filetransfer data WebSocket until close/error.
func Serve(ws *websocket.Conn, p SessionParams) {
	_ = serve(ws, p)
}

func serve(ws *websocket.Conn, p SessionParams) error {
	p.TransferID = strings.TrimSpace(p.TransferID)
	p.Direction = strings.ToLower(strings.TrimSpace(p.Direction))
	p.Path = strings.TrimSpace(p.Path)
	if err := validateTransferID(p.TransferID); err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}
	if p.Path == "" {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "path required"})
	}
	if p.Abort {
		return doAbort(ws, p)
	}
	switch p.Direction {
	case filetransferproto.DirectionUpload:
		return serveUpload(ws, p)
	case filetransferproto.DirectionDownload:
		return serveDownload(ws, p)
	default:
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "direction must be upload or download"})
	}
}

func doAbort(ws *websocket.Conn, p SessionParams) error {
	part, meta := tempPaths(cleanPath(p.Path), p.TransferID)
	removeTemps(part, meta)
	_ = writeCtrl(ws, filetransferproto.Control{
		Type:       filetransferproto.CtrlAborted,
		TransferID: p.TransferID,
		Path:       cleanPath(p.Path),
		OK:         true,
	})
	return nil
}

func serveUpload(ws *websocket.Conn, p SessionParams) error {
	final := cleanPath(p.Path)
	if p.Size < 0 {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "size must be >= 0"})
	}
	part, metaPath := tempPaths(final, p.TransferID)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}

	meta, resumed, err := openOrCreateUpload(part, metaPath, p, final)
	if err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}

	hash := sha256.New()
	if meta.Offset > 0 {
		if err := feedHashFromFile(hash, part, meta.Offset); err != nil {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "cannot resume: " + err.Error()})
		}
	}

	if err := writeCtrl(ws, filetransferproto.Control{
		Type:        filetransferproto.CtrlStatus,
		TransferID:  p.TransferID,
		Direction:   filetransferproto.DirectionUpload,
		Path:        final,
		Size:        p.Size,
		Offset:      meta.Offset,
		Fingerprint: p.Fingerprint,
		OK:          true,
		Resumed:     resumed,
	}); err != nil {
		return err
	}

	f, err := os.OpenFile(part, os.O_WRONLY, 0o644)
	if err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}
	defer f.Close()
	if _, err := f.Seek(meta.Offset, io.SeekStart); err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			// Disconnect = pause; temps kept for resume.
			return err
		}
		if mt == websocket.TextMessage {
			ctrl, err := filetransferproto.DecodeControl(data)
			if err != nil {
				_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "bad control json"})
				continue
			}
			switch ctrl.Type {
			case filetransferproto.CtrlAbort:
				f.Close()
				removeTemps(part, metaPath)
				return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlAborted, TransferID: p.TransferID, OK: true})
			case filetransferproto.CtrlCommit:
				if meta.Offset != p.Size {
					_ = writeCtrl(ws, filetransferproto.Control{
						Type:    filetransferproto.CtrlError,
						Message: fmt.Sprintf("size mismatch: have %d want %d", meta.Offset, p.Size),
					})
					continue
				}
				sum := hex.EncodeToString(hash.Sum(nil))
				if ctrl.SHA256 != "" && !strings.EqualFold(ctrl.SHA256, sum) {
					_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "content sha256 mismatch"})
					continue
				}
				_ = f.Sync()
				f.Close()
				if err := replaceFile(part, final); err != nil {
					return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
				}
				_ = os.Remove(metaPath)
				_ = os.Remove(metaPath + ".tmp")
				return writeCtrl(ws, filetransferproto.Control{
					Type:       filetransferproto.CtrlCommitted,
					TransferID: p.TransferID,
					Path:       final,
					Size:       p.Size,
					SHA256:     sum,
					OK:         true,
				})
			default:
				_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "unexpected control: " + ctrl.Type})
			}
			continue
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		offset, payload, err := filetransferproto.DecodeChunk(data)
		if err != nil {
			_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
			continue
		}
		if offset != meta.Offset {
			// Idempotent replay of last chunk.
			if offset < meta.Offset && offset+int64(len(payload)) <= meta.Offset {
				_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlAck, Offset: meta.Offset, OK: true})
				continue
			}
			_ = writeCtrl(ws, filetransferproto.Control{
				Type:    filetransferproto.CtrlError,
				Message: fmt.Sprintf("unexpected offset %d (want %d)", offset, meta.Offset),
			})
			continue
		}
		if meta.Offset+int64(len(payload)) > p.Size {
			_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "chunk exceeds declared size"})
			continue
		}
		if _, err := f.Write(payload); err != nil {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
		}
		if _, err := hash.Write(payload); err != nil {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
		}
		meta.Offset += int64(len(payload))
		meta.ContentSHA = hex.EncodeToString(hash.Sum(nil))
		if err := writeMetaAtomic(metaPath, meta); err != nil {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
		}
		if err := writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlAck, Offset: meta.Offset, OK: true}); err != nil {
			return err
		}
	}
}

func openOrCreateUpload(part, metaPath string, p SessionParams, final string) (Meta, bool, error) {
	existing, err := readMeta(metaPath)
	if err == nil {
		st, sterr := os.Stat(part)
		if sterr != nil || st.Size() < existing.Offset {
			removeTemps(part, metaPath)
		} else if existing.TransferID == p.TransferID &&
			existing.Path == final &&
			existing.Size == p.Size &&
			existing.Fingerprint == p.Fingerprint {
			// Truncate any uncommitted tail beyond meta.Offset.
			if st.Size() > existing.Offset {
				if err := os.Truncate(part, existing.Offset); err != nil {
					return Meta{}, false, err
				}
			}
			return existing, existing.Offset > 0, nil
		} else {
			return Meta{}, false, fmt.Errorf("existing partial upload does not match fingerprint/size/path")
		}
	} else if !os.IsNotExist(err) {
		return Meta{}, false, err
	}

	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return Meta{}, false, err
	}
	_ = f.Close()
	m := Meta{
		TransferID:  p.TransferID,
		Path:        final,
		Size:        p.Size,
		Fingerprint: p.Fingerprint,
		Offset:      0,
	}
	if err := writeMetaAtomic(metaPath, m); err != nil {
		removeTemps(part, metaPath)
		return Meta{}, false, err
	}
	return m, false, nil
}

func feedHashFromFile(hash io.Writer, path string, n int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(hash, io.LimitReader(f, n))
	return err
}

func serveDownload(ws *websocket.Conn, p SessionParams) error {
	final := cleanPath(p.Path)
	st, err := os.Stat(final)
	if err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}
	if st.IsDir() {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "cannot download a directory"})
	}
	size := st.Size()
	mtime := st.ModTime().Unix()
	fp := fmt.Sprintf("size=%d;mtime=%d", size, mtime)

	if err := writeCtrl(ws, filetransferproto.Control{
		Type:        filetransferproto.CtrlStatus,
		TransferID:  p.TransferID,
		Direction:   filetransferproto.DirectionDownload,
		Path:        final,
		Size:        size,
		Offset:      0,
		Fingerprint: fp,
		Mtime:       mtime,
		OK:          true,
	}); err != nil {
		return err
	}

	f, err := os.Open(final)
	if err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}
	defer f.Close()

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if mt != websocket.TextMessage {
			continue
		}
		ctrl, err := filetransferproto.DecodeControl(data)
		if err != nil {
			_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "bad control json"})
			continue
		}
		switch ctrl.Type {
		case filetransferproto.CtrlAbort:
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlAborted, TransferID: p.TransferID, OK: true})
		case filetransferproto.CtrlRequest:
			if ctrl.Fingerprint != "" && ctrl.Fingerprint != fp {
				return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "source file changed"})
			}
			if ctrl.Offset < 0 || ctrl.Offset > size {
				_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "invalid request offset"})
				continue
			}
			if err := streamDownloadSync(ws, f, ctrl.Offset, size, p.TransferID, fp); err != nil {
				return err
			}
			return nil
		default:
			_ = writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: "unexpected control: " + ctrl.Type})
		}
	}
}

// streamDownloadSync sends chunks and waits for ACK after each until EOF/error/abort.
func streamDownloadSync(ws *websocket.Conn, f *os.File, start, size int64, transferID, fp string) error {
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
	}
	buf := make([]byte, filetransferproto.MaxChunk)
	offset := start
	for offset < size {
		n := int64(len(buf))
		if size-offset < n {
			n = size - offset
		}
		readN, err := io.ReadFull(f, buf[:n])
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
		}
		if readN == 0 {
			break
		}
		frame, err := filetransferproto.EncodeChunk(offset, buf[:readN])
		if err != nil {
			return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlError, Message: err.Error()})
		}
		if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return err
		}
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return err
			}
			if mt != websocket.TextMessage {
				continue
			}
			ctrl, err := filetransferproto.DecodeControl(data)
			if err != nil {
				continue
			}
			if ctrl.Type == filetransferproto.CtrlAbort {
				return writeCtrl(ws, filetransferproto.Control{Type: filetransferproto.CtrlAborted, TransferID: transferID, OK: true})
			}
			if ctrl.Type == filetransferproto.CtrlAck {
				if ctrl.Offset != offset+int64(readN) {
					return writeCtrl(ws, filetransferproto.Control{
						Type:    filetransferproto.CtrlError,
						Message: fmt.Sprintf("bad ack offset %d", ctrl.Offset),
					})
				}
				break
			}
			if ctrl.Type == filetransferproto.CtrlError {
				return fmt.Errorf("%s", ctrl.Message)
			}
		}
		offset += int64(readN)
	}
	return writeCtrl(ws, filetransferproto.Control{
		Type:        filetransferproto.CtrlCommitted,
		TransferID:  transferID,
		Size:        size,
		Offset:      offset,
		Fingerprint: fp,
		OK:          true,
	})
}

func writeCtrl(ws *websocket.Conn, c filetransferproto.Control) error {
	raw, err := filetransferproto.EncodeControl(c)
	if err != nil {
		return err
	}
	return ws.WriteMessage(websocket.TextMessage, raw)
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	return filepath.Clean(p)
}

// AbortTransfer cleans temps for a transfer without a data session (control-plane).
func AbortTransfer(path, transferID string) error {
	if err := validateTransferID(transferID); err != nil {
		return err
	}
	part, meta := tempPaths(cleanPath(path), transferID)
	removeTemps(part, meta)
	return nil
}
