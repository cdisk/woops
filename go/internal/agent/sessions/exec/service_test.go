package exec

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestBuildCmdLinuxOrWindows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := buildCmd(ctx, "echo hi", "")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if cmd.Path == "" && cmd.Args[0] != "powershell.exe" {
			// Args[0] is program name for CommandContext
		}
		if len(cmd.Args) < 2 || cmd.Args[0] != "powershell.exe" {
			t.Fatalf("expected powershell, got %v", cmd.Args)
		}
	} else {
		if len(cmd.Args) < 3 || cmd.Args[0] != "bash" {
			t.Fatalf("expected bash -lc, got %v", cmd.Args)
		}
	}
}

func TestBuildCmdInvalidCwd(t *testing.T) {
	ctx := context.Background()
	_, err := buildCmd(ctx, "echo hi", "/nonexistent/path/opsctl-test-xyz")
	if err == nil {
		t.Fatal("expected invalid cwd error")
	}
}
