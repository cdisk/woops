package filetransfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/websocket"
	filetransferproto "github.com/ops-bastion/ops/go/internal/protocol/filetransfer"
)

func TestUploadCommitAndResume(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "out.bin")
	payload := bytes.Repeat([]byte("0123456789"), 200_000) // ~2MB
	half := len(payload) / 2
	fp := "fp-test"
	tid := "11111111-1111-1111-1111-111111111111"

	// First session: write first half then disconnect.
	client, cleanup := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "upload", Path: final,
		Size: int64(len(payload)), Fingerprint: fp,
	})
	defer cleanup()

	status := readCtrl(t, client)
	if status.Type != filetransferproto.CtrlStatus || status.Offset != 0 {
		t.Fatalf("status=%+v", status)
	}
	frame, err := filetransferproto.EncodeChunk(0, payload[:half])
	if err != nil {
		t.Fatal(err)
	}
	if err := client.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	ack := readCtrl(t, client)
	if ack.Type != filetransferproto.CtrlAck || ack.Offset != int64(half) {
		t.Fatalf("ack=%+v", ack)
	}
	_ = client.Close()
	cleanup()

	part, meta := tempPaths(final, tid)
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("part missing: %v", err)
	}
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("meta missing: %v", err)
	}

	// Resume second half + commit.
	client2, cleanup2 := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "upload", Path: final,
		Size: int64(len(payload)), Fingerprint: fp,
	})
	defer cleanup2()
	status2 := readCtrl(t, client2)
	if !status2.Resumed || status2.Offset != int64(half) {
		t.Fatalf("resume status=%+v", status2)
	}
	frame2, err := filetransferproto.EncodeChunk(int64(half), payload[half:])
	if err != nil {
		t.Fatal(err)
	}
	if err := client2.WriteMessage(websocket.BinaryMessage, frame2); err != nil {
		t.Fatal(err)
	}
	ack2 := readCtrl(t, client2)
	if ack2.Offset != int64(len(payload)) {
		t.Fatalf("ack2=%+v", ack2)
	}
	sum := sha256.Sum256(payload)
	raw, _ := filetransferproto.EncodeControl(filetransferproto.Control{
		Type: filetransferproto.CtrlCommit, SHA256: hex.EncodeToString(sum[:]),
	})
	if err := client2.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatal(err)
	}
	done := readCtrl(t, client2)
	if done.Type != filetransferproto.CtrlCommitted || !done.OK {
		t.Fatalf("commit=%+v", done)
	}
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("final content mismatch")
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("part should be gone")
	}
}

func TestUploadAbortCleansTemps(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "x.bin")
	tid := "22222222-2222-2222-2222-222222222222"
	client, cleanup := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "upload", Path: final, Size: 100, Fingerprint: "a",
	})
	defer cleanup()
	_ = readCtrl(t, client)
	frame, _ := filetransferproto.EncodeChunk(0, bytes.Repeat([]byte("a"), 50))
	_ = client.WriteMessage(websocket.BinaryMessage, frame)
	_ = readCtrl(t, client)
	raw, _ := filetransferproto.EncodeControl(filetransferproto.Control{Type: filetransferproto.CtrlAbort})
	_ = client.WriteMessage(websocket.TextMessage, raw)
	ab := readCtrl(t, client)
	if ab.Type != filetransferproto.CtrlAborted {
		t.Fatalf("aborted=%+v", ab)
	}
	part, meta := tempPaths(final, tid)
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("part should be removed")
	}
	if _, err := os.Stat(meta); !os.IsNotExist(err) {
		t.Fatal("meta should be removed")
	}
}

func TestDownloadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "src.bin")
	payload := bytes.Repeat([]byte("xyz"), 400_000)
	if err := os.WriteFile(final, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	tid := "33333333-3333-3333-3333-333333333333"
	client, cleanup := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "download", Path: final,
	})
	defer cleanup()
	st := readCtrl(t, client)
	if st.Type != filetransferproto.CtrlStatus || st.Size != int64(len(payload)) {
		t.Fatalf("status=%+v", st)
	}
	req, _ := filetransferproto.EncodeControl(filetransferproto.Control{
		Type: filetransferproto.CtrlRequest, Offset: 0, Fingerprint: st.Fingerprint,
	})
	if err := client.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatal(err)
	}
	var out []byte
	for {
		mt, data, err := client.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt == websocket.TextMessage {
			ctrl, err := filetransferproto.DecodeControl(data)
			if err != nil {
				t.Fatal(err)
			}
			if ctrl.Type == filetransferproto.CtrlCommitted {
				break
			}
			if ctrl.Type == filetransferproto.CtrlError {
				t.Fatalf("error: %s", ctrl.Message)
			}
			continue
		}
		off, chunk, err := filetransferproto.DecodeChunk(data)
		if err != nil {
			t.Fatal(err)
		}
		if off != int64(len(out)) {
			t.Fatalf("offset=%d want=%d", off, len(out))
		}
		out = append(out, chunk...)
		ack, _ := filetransferproto.EncodeControl(filetransferproto.Control{
			Type: filetransferproto.CtrlAck, Offset: int64(len(out)), OK: true,
		})
		if err := client.WriteMessage(websocket.TextMessage, ack); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("download mismatch")
	}
}

func TestFingerprintMismatchRejectsResume(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "f.bin")
	tid := "44444444-4444-4444-4444-444444444444"
	client, cleanup := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "upload", Path: final, Size: 10, Fingerprint: "one",
	})
	_ = readCtrl(t, client)
	frame, _ := filetransferproto.EncodeChunk(0, []byte("0123456789"))
	_ = client.WriteMessage(websocket.BinaryMessage, frame)
	_ = readCtrl(t, client)
	_ = client.Close()
	cleanup()

	client2, cleanup2 := dialXfer(t, SessionParams{
		TransferID: tid, Direction: "upload", Path: final, Size: 10, Fingerprint: "two",
	})
	defer cleanup2()
	msg := readCtrl(t, client2)
	if msg.Type != filetransferproto.CtrlError {
		t.Fatalf("want error, got %+v", msg)
	}
}

func dialXfer(t *testing.T, p SessionParams) (*websocket.Conn, func()) {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		Serve(ws, p)
		_ = ws.Close()
	}))
	url := "ws" + srv.URL[len("http"):]
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return ws, func() {
		_ = ws.Close()
		srv.Close()
	}
}

func readCtrl(t *testing.T, ws *websocket.Conn) filetransferproto.Control {
	t.Helper()
	mt, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("want text, got mt=%d", mt)
	}
	c, err := filetransferproto.DecodeControl(data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
