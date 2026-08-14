package opsctl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gorilla/websocket"
	filetransferproto "github.com/ops-bastion/ops/go/internal/protocol/filetransfer"
)

const (
	xferChunk      = filetransferproto.MaxChunk
	xferMaxRetries = 8
)

// TransferTicket is the subset of ticket fields needed for filetransfer.
type TransferTicket struct {
	BrowserWS   string
	TransferID  string
	Direction   string
	Path        string
	Size        int64
	Fingerprint string
	Abort       bool
}

// ProgressPhase is a human-oriented transfer stage for CLI / UI reporters.
type ProgressPhase string

const (
	ProgressTicket     ProgressPhase = "ticket"
	ProgressConnecting ProgressPhase = "connecting"
	ProgressConnected  ProgressPhase = "connected"
	ProgressTransfer   ProgressPhase = "transfer"
	ProgressRetrying   ProgressPhase = "retrying"
	ProgressDone       ProgressPhase = "done"
)

// TransferProgress reports upload/download status. Loaded/Total are byte counts.
type TransferProgress struct {
	Phase   ProgressPhase
	Loaded  int64
	Total   int64
	Resumed bool
	Attempt int // 1-based retry counter
	Message string
}

// ProgressFunc receives progress updates; may be nil.
type ProgressFunc func(TransferProgress)

func report(fn ProgressFunc, p TransferProgress) {
	if fn != nil {
		fn(p)
	}
}

// UploadBinary uploads localPath to remotePath over a filetransfer session,
// automatically reconnecting from the Agent's durable offset on transient failures.
func UploadBinary(
	cfg Config,
	issue func(transferID, fingerprint string, size int64, abort bool) (*TransferTicket, error),
	localPath, remotePath string,
	onProgress ProgressFunc,
) error {
	st, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("local path is a directory: %s", localPath)
	}
	fp, err := localFingerprint(localPath, st)
	if err != nil {
		return err
	}
	transferID := ""
	var lastErr error
	for attempt := 0; attempt < xferMaxRetries; attempt++ {
		if attempt > 0 {
			report(onProgress, TransferProgress{
				Phase: ProgressRetrying, Attempt: attempt + 1, Total: st.Size(), Message: lastErr.Error(),
			})
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
		report(onProgress, TransferProgress{Phase: ProgressTicket, Attempt: attempt + 1, Total: st.Size()})
		ticket, err := issue(transferID, fp, st.Size(), false)
		if err != nil {
			lastErr = err
			continue
		}
		if transferID == "" {
			transferID = ticket.TransferID
		}
		done, err := uploadOnce(cfg, ticket, localPath, st.Size(), attempt+1, onProgress)
		if done {
			report(onProgress, TransferProgress{Phase: ProgressDone, Loaded: st.Size(), Total: st.Size(), Attempt: attempt + 1})
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("upload failed after retries")
	}
	return lastErr
}

// DownloadBinary downloads remotePath to localPath with reconnect/resume.
func DownloadBinary(
	cfg Config,
	issue func(transferID string, abort bool) (*TransferTicket, error),
	remotePath, localPath string,
	onProgress ProgressFunc,
) error {
	transferID := ""
	var lastErr error
	var localOffset int64
	if st, err := os.Stat(localPath); err == nil && !st.IsDir() {
		localOffset = st.Size()
	}
	for attempt := 0; attempt < xferMaxRetries; attempt++ {
		if attempt > 0 {
			report(onProgress, TransferProgress{
				Phase: ProgressRetrying, Attempt: attempt + 1, Loaded: localOffset, Message: lastErr.Error(),
			})
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
		report(onProgress, TransferProgress{Phase: ProgressTicket, Attempt: attempt + 1, Loaded: localOffset})
		ticket, err := issue(transferID, false)
		if err != nil {
			lastErr = err
			continue
		}
		if transferID == "" {
			transferID = ticket.TransferID
		}
		done, nextOff, err := downloadOnce(cfg, ticket, localPath, localOffset, attempt+1, onProgress)
		localOffset = nextOff
		if done {
			report(onProgress, TransferProgress{Phase: ProgressDone, Loaded: localOffset, Total: localOffset, Attempt: attempt + 1})
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download failed after retries")
	}
	return lastErr
}

// AbortTransfer asks the Agent to discard temps for transferID/path.
func AbortTransfer(cfg Config, ticket *TransferTicket) error {
	if ticket == nil || ticket.BrowserWS == "" {
		return fmt.Errorf("missing ticket")
	}
	dialer := cfg.dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	ws, _, err := dialer.Dial(ticket.BrowserWS, nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	mt, data, err := ws.ReadMessage()
	if err != nil {
		return err
	}
	if mt == websocket.TextMessage {
		ctrl, _ := filetransferproto.DecodeControl(data)
		if ctrl.Type == filetransferproto.CtrlAborted || ctrl.Type == filetransferproto.CtrlError {
			return nil
		}
	}
	return nil
}

func uploadOnce(cfg Config, ticket *TransferTicket, localPath string, size int64, attempt int, onProgress ProgressFunc) (done bool, err error) {
	dialer := cfg.dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	report(onProgress, TransferProgress{Phase: ProgressConnecting, Attempt: attempt, Total: size})
	ws, _, err := dialer.Dial(ticket.BrowserWS, nil)
	if err != nil {
		return false, err
	}
	defer ws.Close()

	status, err := readControl(ws)
	if err != nil {
		return false, err
	}
	if status.Type == filetransferproto.CtrlError {
		return false, fmt.Errorf("%s", status.Message)
	}
	if status.Type != filetransferproto.CtrlStatus {
		return false, fmt.Errorf("unexpected control %s", status.Type)
	}
	offset := status.Offset
	if offset > size {
		return false, fmt.Errorf("remote offset %d exceeds local size %d", offset, size)
	}
	resumed := offset > 0
	report(onProgress, TransferProgress{
		Phase: ProgressConnected, Loaded: offset, Total: size, Resumed: resumed, Attempt: attempt,
	})

	f, err := os.Open(localPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false, err
	}

	hash := sha256.New()
	if offset > 0 {
		hf, err := os.Open(localPath)
		if err != nil {
			return false, err
		}
		_, copyErr := io.Copy(hash, io.LimitReader(hf, offset))
		_ = hf.Close()
		if copyErr != nil {
			return false, copyErr
		}
	}

	buf := make([]byte, xferChunk)
	for offset < size {
		n, readErr := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			frame, err := filetransferproto.EncodeChunk(offset, chunk)
			if err != nil {
				return false, err
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return false, err
			}
			ack, err := readControl(ws)
			if err != nil {
				return false, err
			}
			if ack.Type == filetransferproto.CtrlError {
				return false, fmt.Errorf("%s", ack.Message)
			}
			if ack.Type != filetransferproto.CtrlAck || ack.Offset != offset+int64(n) {
				return false, fmt.Errorf("bad ack: %+v", ack)
			}
			_, _ = hash.Write(chunk)
			offset = ack.Offset
			report(onProgress, TransferProgress{
				Phase: ProgressTransfer, Loaded: offset, Total: size, Resumed: resumed, Attempt: attempt,
			})
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return false, readErr
		}
	}

	if offset != size {
		return false, fmt.Errorf("short read: have %d want %d", offset, size)
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	raw, _ := filetransferproto.EncodeControl(filetransferproto.Control{Type: filetransferproto.CtrlCommit, SHA256: sum})
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		return false, err
	}
	doneCtrl, err := readControl(ws)
	if err != nil {
		return false, err
	}
	if doneCtrl.Type == filetransferproto.CtrlError {
		return false, fmt.Errorf("%s", doneCtrl.Message)
	}
	if doneCtrl.Type != filetransferproto.CtrlCommitted || !doneCtrl.OK {
		return false, fmt.Errorf("commit failed: %+v", doneCtrl)
	}
	return true, nil
}

func downloadOnce(cfg Config, ticket *TransferTicket, localPath string, startOffset int64, attempt int, onProgress ProgressFunc) (done bool, nextOffset int64, err error) {
	dialer := cfg.dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	report(onProgress, TransferProgress{Phase: ProgressConnecting, Attempt: attempt, Loaded: startOffset})
	ws, _, err := dialer.Dial(ticket.BrowserWS, nil)
	if err != nil {
		return false, startOffset, err
	}
	defer ws.Close()

	status, err := readControl(ws)
	if err != nil {
		return false, startOffset, err
	}
	if status.Type == filetransferproto.CtrlError {
		return false, startOffset, fmt.Errorf("%s", status.Message)
	}
	if status.Type != filetransferproto.CtrlStatus {
		return false, startOffset, fmt.Errorf("unexpected control %s", status.Type)
	}
	size := status.Size
	if startOffset > size {
		startOffset = 0
	}
	resumed := startOffset > 0
	report(onProgress, TransferProgress{
		Phase: ProgressConnected, Loaded: startOffset, Total: size, Resumed: resumed, Attempt: attempt,
	})
	flags := os.O_CREATE | os.O_WRONLY
	if startOffset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(localPath, flags, 0o644)
	if err != nil {
		return false, startOffset, err
	}
	defer f.Close()
	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return false, startOffset, err
	}

	req, _ := filetransferproto.EncodeControl(filetransferproto.Control{
		Type: filetransferproto.CtrlRequest, Offset: startOffset, Fingerprint: status.Fingerprint,
	})
	if err := ws.WriteMessage(websocket.TextMessage, req); err != nil {
		return false, startOffset, err
	}

	offset := startOffset
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return false, offset, err
		}
		if mt == websocket.TextMessage {
			ctrl, err := filetransferproto.DecodeControl(data)
			if err != nil {
				return false, offset, err
			}
			switch ctrl.Type {
			case filetransferproto.CtrlCommitted:
				report(onProgress, TransferProgress{
					Phase: ProgressTransfer, Loaded: offset, Total: size, Resumed: resumed, Attempt: attempt,
				})
				return true, offset, nil
			case filetransferproto.CtrlError:
				return false, offset, fmt.Errorf("%s", ctrl.Message)
			case filetransferproto.CtrlAborted:
				return false, offset, fmt.Errorf("aborted")
			default:
				continue
			}
		}
		off, chunk, err := filetransferproto.DecodeChunk(data)
		if err != nil {
			return false, offset, err
		}
		if off != offset {
			return false, offset, fmt.Errorf("unexpected download offset %d want %d", off, offset)
		}
		if _, err := f.Write(chunk); err != nil {
			return false, offset, err
		}
		offset += int64(len(chunk))
		ack, _ := filetransferproto.EncodeControl(filetransferproto.Control{Type: filetransferproto.CtrlAck, Offset: offset, OK: true})
		if err := ws.WriteMessage(websocket.TextMessage, ack); err != nil {
			return false, offset, err
		}
		report(onProgress, TransferProgress{
			Phase: ProgressTransfer, Loaded: offset, Total: size, Resumed: resumed, Attempt: attempt,
		})
	}
}

func readControl(ws *websocket.Conn) (filetransferproto.Control, error) {
	mt, data, err := ws.ReadMessage()
	if err != nil {
		return filetransferproto.Control{}, err
	}
	if mt != websocket.TextMessage {
		if len(data) > 0 && data[0] != '{' {
			return filetransferproto.Control{}, fmt.Errorf("%s", string(data))
		}
		return filetransferproto.Control{}, fmt.Errorf("expected control text frame")
	}
	if len(data) > 0 && data[0] != '{' {
		return filetransferproto.Control{}, fmt.Errorf("%s", string(data))
	}
	return filetransferproto.DecodeControl(data)
}

func localFingerprint(path string, st os.FileInfo) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const n = 64 * 1024
	head := make([]byte, n)
	hn, _ := io.ReadFull(f, head)
	head = head[:max(0, hn)]
	var tail []byte
	if st.Size() > n {
		tail = make([]byte, n)
		if _, err := f.Seek(st.Size()-n, io.SeekStart); err != nil {
			return "", err
		}
		tn, _ := io.ReadFull(f, tail)
		tail = tail[:max(0, tn)]
	} else {
		tail = head
	}
	h := sha256.New()
	_, _ = h.Write(head)
	_, _ = h.Write(tail)
	sum := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("v1;size=%d;mtime=%d;hash=%s", st.Size(), st.ModTime().UnixMilli(), sum), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
