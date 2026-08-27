//go:build windows

package shell

import (
	"strings"
	"testing"

	"github.com/UserExistsError/conpty"
)

func TestBuildWindowsCmdLine(t *testing.T) {
	t.Parallel()
	line, kind := buildWindowsCmdLine("cmd", `C:\Users\test`)
	if kind != "cmd" {
		t.Fatalf("kind=%q want cmd", kind)
	}
	if !strings.Contains(strings.ToLower(line), "cmd.exe") {
		t.Fatalf("line=%q", line)
	}
	if !strings.Contains(line, "65001") {
		t.Fatalf("expected chcp 65001 in %q", line)
	}

	line, kind = buildWindowsCmdLine("powershell", `C:\Users\test`)
	if kind != "powershell" {
		t.Fatalf("kind=%q want powershell", kind)
	}
	if !strings.Contains(strings.ToLower(line), "powershell.exe") {
		t.Fatalf("line=%q", line)
	}
}

func TestEffectiveShellKindWithoutConPTY(t *testing.T) {
	t.Parallel()
	if conpty.IsConPtyAvailable() {
		t.Skip("ConPTY available on this host")
	}
	for _, in := range []string{"powershell", "cmd", ""} {
		if got := effectiveShellKind(in); got != "cmd" {
			t.Fatalf("effectiveShellKind(%q)=%q want cmd", in, got)
		}
	}
}
