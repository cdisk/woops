package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestInstallBinaryAndRestore(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	dst := filepath.Join(dir, "woops-agent.exe")
	old := dst + ".old"
	if err := os.WriteFile(src, []byte("new-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallBinary(src, dst, old); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "new-bin" {
		t.Fatalf("dst=%q", got)
	}
	bak, _ := os.ReadFile(old)
	if string(bak) != "old-bin" {
		t.Fatalf("old=%q", bak)
	}
	if err := RestoreBinary(dst, old); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(dst)
	if string(got) != "old-bin" {
		t.Fatalf("restored=%q", got)
	}
}

func TestWaitRegistered(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		ConfDir:  dir,
		AssetID:  filepath.Join(dir, "asset-id"),
		Token:    filepath.Join(dir, "agent-token"),
		CodeFile: filepath.Join(dir, "install-code"),
	}
	_ = os.WriteFile(p.CodeFile, []byte("CODE\n"), 0o600)
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(p.AssetID, []byte("id\n"), 0o644)
		_ = os.WriteFile(p.Token, []byte("tok\n"), 0o644)
		_ = os.Remove(p.CodeFile)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitRegistered(ctx, p, 2*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWriteInstallCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-code ACL is SYSTEM/Administrators-only; needs elevation to verify")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "install-code")
	if err := WriteInstallCode(path, "abc123"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc123\n" {
		t.Fatalf("got %q", got)
	}
	if err := WriteInstallCode(path, "xyz"); err != nil {
		t.Fatal(err)
	}
}
