package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordingDetailPreservesReplayMetadata(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "recordings", "operation-1")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("guacd recording")
	if err := os.WriteFile(filepath.Join(dir, guacRecordingName), content, 0o640); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	detail := recordingDetail(Deps{
		RecordingRelPath: func(path string) string {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.ToSlash(rel)
		},
	}, dir)
	if detail["recordingPath"] != "recordings/operation-1/session" ||
		detail["format"] != "guacamole" ||
		detail["sha256"] != hex.EncodeToString(sum[:]) ||
		detail["size"] != int64(len(content)) {
		t.Fatalf("recording detail: %#v", detail)
	}
}
