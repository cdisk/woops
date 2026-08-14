package shell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartRecordingUsesOperationDirectory(t *testing.T) {
	root := t.TempDir()
	deps := Deps{
		RecordingDir: func(operationID string) (string, error) {
			dir := filepath.Join(root, "recordings", operationID)
			return dir, os.MkdirAll(dir, 0o750)
		},
		RecordingRelPath: func(path string) string {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.ToSlash(rel)
		},
	}
	rec, rel := startRecording(deps, "operation-1", "shell title")
	if rec == nil {
		t.Fatal("recording not created")
	}
	rec.WriteOutput([]byte("hello"))
	sum, size, err := rec.Close()
	if err != nil {
		t.Fatal(err)
	}
	if rel != "recordings/operation-1/session.cast" || sum == "" || size <= 0 {
		t.Fatalf("recording metadata: path=%q sha256=%q size=%d", rel, sum, size)
	}
}
