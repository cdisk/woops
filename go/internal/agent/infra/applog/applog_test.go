package applog

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Cleanup(func() { dirOverride = "" })
	dirOverride = ""
	got := ResolveDir(filepath.Join("C:", "ProgramData", "woops-agent", "agent.yaml"))
	if runtime.GOOS == "windows" {
		if !strings.Contains(strings.ToLower(got), "woops-agent") {
			t.Fatalf("windows ResolveDir=%q", got)
		}
		return
	}
	if got != "/var/log/woops-agent" {
		t.Fatalf("linux ResolveDir=%q", got)
	}
}

func TestNewWritesAndMirrors(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { dirOverride = "" })
	dirOverride = dir
	cfg := filepath.Join(dir, "agent.yaml")
	l, closer, err := New(cfg, "woops-agent.log")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	l.Printf("op START type=SHELL session=test")
	b, err := os.ReadFile(filepath.Join(dir, "woops-agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "op START type=SHELL session=test") {
		t.Fatalf("log content=%q", b)
	}
}

func TestSetupDefaultSetsStdLog(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { dirOverride = "" })
	dirOverride = dir
	cfg := filepath.Join(dir, "agent.yaml")
	closer, err := SetupDefault(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	log.Printf("hello-applog")
	b, err := os.ReadFile(filepath.Join(dir, "woops-agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hello-applog") {
		t.Fatalf("got %q", b)
	}
}
