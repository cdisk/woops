package agentinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentBinaryPrefersWoops(t *testing.T) {
	dir := t.TempDir()
	woops := filepath.Join(dir, "woops-agent-linux-amd64")
	if err := os.WriteFile(woops, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, name := resolveAgentBinary(dir, "linux", "amd64")
	if path != woops || name != "woops-agent-linux-amd64" {
		t.Fatalf("got path=%q name=%q", path, name)
	}
}

func TestResolveAgentBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	// Legacy ops-agent name must not be resolved anymore.
	legacy := filepath.Join(dir, "ops-agent-linux-amd64")
	if err := os.WriteFile(legacy, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, name := resolveAgentBinary(dir, "linux", "amd64")
	if path != "" {
		t.Fatalf("expected empty path for missing woops-agent, got path=%q name=%q", path, name)
	}
	if name != "woops-agent-linux-amd64" {
		t.Fatalf("display name want woops-agent-linux-amd64 got %q", name)
	}
}

func TestResolveAgentBinaryWindowsExe(t *testing.T) {
	dir := t.TempDir()
	woops := filepath.Join(dir, "woops-agent-windows-amd64.exe")
	if err := os.WriteFile(woops, []byte("win"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, name := resolveAgentBinary(dir, "windows", "amd64")
	if path != woops || name != "woops-agent-windows-amd64.exe" {
		t.Fatalf("got path=%q name=%q", path, name)
	}
}

func TestResolveWoopsctlBinary(t *testing.T) {
	dir := t.TempDir()
	linux := filepath.Join(dir, "woopsctl-linux-amd64")
	win := filepath.Join(dir, "woopsctl-windows-amd64.exe")
	if err := os.WriteFile(linux, []byte("ctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(win, []byte("ctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, name := resolveWoopsctlBinary(dir, "linux", "amd64")
	if path != linux || name != "woopsctl-linux-amd64" {
		t.Fatalf("linux: path=%q name=%q", path, name)
	}
	path, name = resolveWoopsctlBinary(dir, "windows", "amd64")
	if path != win || name != "woopsctl-windows-amd64.exe" {
		t.Fatalf("windows: path=%q name=%q", path, name)
	}
	path, name = resolveWoopsctlBinary(dir, "linux", "arm64")
	if path != "" || name != "woopsctl-linux-arm64" {
		t.Fatalf("missing: path=%q name=%q", path, name)
	}
}

func TestValidWoopsctlPlatform(t *testing.T) {
	if !validWoopsctlPlatform("linux", "amd64") || !validWoopsctlPlatform("windows", "arm64") {
		t.Fatal("expected supported platforms")
	}
	if validWoopsctlPlatform("freebsd", "amd64") || validWoopsctlPlatform("linux", "386") {
		t.Fatal("expected unsupported platforms rejected")
	}
}

func TestResolveWinptyFile(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	tp := filepath.Join(root, "third_party", "winpty", "x64")
	if err := os.MkdirAll(filepath.Join(binDir, "winpty", "amd64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tp, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(binDir, "winpty", "amd64", "winpty.dll"))
	mustWrite(filepath.Join(tp, "winpty-agent.exe"))

	got := resolveWinptyFile(binDir, "amd64", "winpty.dll")
	want := filepath.Join(binDir, "winpty", "amd64", "winpty.dll")
	if got != want {
		t.Fatalf("prefer bin dir: got %q want %q", got, want)
	}
	got = resolveWinptyFile(binDir, "amd64", "winpty-agent.exe")
	want = filepath.Join(tp, "winpty-agent.exe")
	if got != want {
		t.Fatalf("third_party fallback: got %q want %q", got, want)
	}
	if resolveWinptyFile(binDir, "amd64", "nope.exe") != "" {
		t.Fatal("expected empty for bad filename path")
	}
}
