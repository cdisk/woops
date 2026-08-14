package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAppendWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "test-instance")
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := w.Append(map[string]any{"phase": PhaseAction, "seq": i}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	segments := sealedSegments(t, dir)
	if len(segments) != 1 {
		t.Fatalf("want 1 sealed segment, got %d (%v)", len(segments), segments)
	}
	data, err := os.ReadFile(segments[0])
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d not json: %v", i, err)
		}
		if m["phase"] != PhaseAction {
			t.Fatalf("line %d phase=%v", i, m["phase"])
		}
	}
}

func TestAppendRotatesOnSizeLimit(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "rotate")
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	// Force rotation without writing 64 MiB.
	w.mu.Lock()
	w.written = SegmentMaxBytes
	w.mu.Unlock()

	if err := w.Append(map[string]any{"phase": PhaseStart}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	segments := sealedSegments(t, dir)
	if len(segments) != 2 {
		t.Fatalf("want 2 sealed segments after rotation, got %d (%v)", len(segments), segments)
	}
}

func TestNewWriterSkipsExistingSequences(t *testing.T) {
	dir := t.TempDir()
	first, err := NewWriter(dir, "restart")
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := first.Append(map[string]any{"phase": PhaseStart}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Simulate a crash: segment stays .open.
	second, err := NewWriter(dir, "restart")
	if err != nil {
		t.Fatalf("NewWriter after restart: %v", err)
	}
	if second.path == first.path {
		t.Fatalf("restarted writer reused active segment %s", first.path)
	}
	_ = first.Close()
	_ = second.Close()
}

func TestRecordingDir(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "rec")
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()
	got, err := w.RecordingDir("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("RecordingDir: %v", err)
	}
	now := time.Now().UTC()
	want := filepath.Join(w.Dir(), now.Format("2006"), now.Format("01"), now.Format("02"),
		"recordings", "11111111-2222-3333-4444-555555555555")
	if got != want {
		t.Fatalf("RecordingDir = %s, want %s", got, want)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("recording dir not created: %v", err)
	}
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(got); err != nil || st.Mode().Perm()&0o007 != 0o007 {
			t.Fatalf("recording dir should be world-writable for guacd, mode=%v err=%v", st.Mode(), err)
		}
		day := filepath.Dir(filepath.Dir(got)) // .../yyyy/mm/dd
		if st, err := os.Stat(day); err != nil || st.Mode().Perm()&0o005 != 0o005 {
			t.Fatalf("day dir should be traversable by others, mode=%v err=%v", st.Mode(), err)
		}
	}
	if _, err := w.RecordingDir(""); err == nil {
		t.Fatal("empty operationId should fail")
	}
}

func sealedSegments(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return out
}
