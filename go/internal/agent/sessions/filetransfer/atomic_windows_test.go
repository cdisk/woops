//go:build windows

package filetransfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.ini")
	tmp := filepath.Join(dir, "upload.part")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(tmp, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("destination = %q, want new", got)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temporary file should be gone, stat err = %v", err)
	}
}

func TestReplaceFileFailurePreservesExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.ini")
	missingTmp := filepath.Join(dir, "missing.part")
	if err := os.WriteFile(dest, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(missingTmp, dest); err == nil {
		t.Fatal("expected replacement to fail")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("destination was lost after failed replacement: %v", err)
	}
	if string(got) != "keep" {
		t.Fatalf("destination = %q, want keep", got)
	}
}
