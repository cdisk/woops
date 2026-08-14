package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCastRecorderWritesHeaderAndEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.cast")
	rec, err := NewCastRecorder(path, "powershell alice@asset")
	if err != nil {
		t.Fatalf("NewCastRecorder: %v", err)
	}
	rec.WriteResize(100, 30)
	rec.WriteInput([]byte("ls\r"))
	rec.WriteOutput([]byte("total 0\r\n"))
	sum, size, err := rec.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	if int64(len(data)) != size {
		t.Fatalf("Close size = %d, file is %d bytes", size, len(data))
	}
	want := sha256.Sum256(data)
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("Close sha256 = %s, want %s", sum, hex.EncodeToString(want[:]))
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 events, got %d lines: %q", len(lines), lines)
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header not json: %v", err)
	}
	if header["version"] != float64(2) || header["width"] != float64(castWidth) {
		t.Fatalf("unexpected header: %v", header)
	}
	if header["title"] != "powershell alice@asset" {
		t.Fatalf("header title = %v", header["title"])
	}
	wantEvents := []struct{ kind, data string }{
		{"r", "100x30"},
		{"i", "ls\r"},
		{"o", "total 0\r\n"},
	}
	for i, want := range wantEvents {
		var ev []any
		if err := json.Unmarshal([]byte(lines[i+1]), &ev); err != nil {
			t.Fatalf("event %d not json: %v", i, err)
		}
		if len(ev) != 3 {
			t.Fatalf("event %d has %d fields", i, len(ev))
		}
		if _, ok := ev[0].(float64); !ok {
			t.Fatalf("event %d timestamp = %v", i, ev[0])
		}
		if ev[1] != want.kind || ev[2] != want.data {
			t.Fatalf("event %d = %v %v, want %s %q", i, ev[1], ev[2], want.kind, want.data)
		}
	}
}

func TestCastRecorderJoinsSplitRunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.cast")
	rec, err := NewCastRecorder(path, "")
	if err != nil {
		t.Fatalf("NewCastRecorder: %v", err)
	}
	// "中文" split across frames, the second frame ending mid-rune.
	full := []byte("中文abc")
	rec.WriteOutput(full[:1])
	rec.WriteOutput(full[1:5])
	rec.WriteOutput(full[5:])
	if _, _, err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	var got strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n")[1:] {
		var ev []any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("event not json: %v", err)
		}
		got.WriteString(ev[2].(string))
	}
	if got.String() != string(full) {
		t.Fatalf("recorded %q, want %q", got.String(), full)
	}
}

func TestCastRecorderNilAndDoubleClose(t *testing.T) {
	var nilRec *CastRecorder
	nilRec.WriteOutput([]byte("x"))
	nilRec.WriteInput([]byte("y"))
	nilRec.WriteResize(80, 24)
	if sum, size, err := nilRec.Close(); sum != "" || size != 0 || err != nil {
		t.Fatalf("nil Close = %q %d %v", sum, size, err)
	}

	path := filepath.Join(t.TempDir(), "session.cast")
	rec, err := NewCastRecorder(path, "")
	if err != nil {
		t.Fatalf("NewCastRecorder: %v", err)
	}
	rec.WriteOutput([]byte("hi"))
	sum, size, err := rec.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Writes after Close are dropped and the result repeats.
	rec.WriteOutput([]byte("ignored"))
	sum2, size2, err := rec.Close()
	if sum2 != sum || size2 != size || err != nil {
		t.Fatalf("second Close = %q %d %v, want %q %d nil", sum2, size2, err, sum, size)
	}
}

func TestWriterRelPath(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "rel")
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()
	abs := filepath.Join(w.Dir(), "2026", "08", "03", "recordings", "op", "session.cast")
	if got := w.RelPath(abs); got != "2026/08/03/recordings/op/session.cast" {
		t.Fatalf("RelPath = %q", got)
	}
	if got := w.RelPath(filepath.Join(w.Dir(), "..", "escape")); got != "" {
		t.Fatalf("RelPath outside spool = %q, want empty", got)
	}
}
