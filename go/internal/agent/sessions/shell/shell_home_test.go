package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnvWithHomeOverridesPWD(t *testing.T) {
	home := "/root"
	if runtime.GOOS == "windows" {
		home = `C:\Users\Administrator`
	}
	env := envWithHome([]string{
		"PATH=/usr/bin",
		"PWD=/etc/woops-agent",
		"HOME=/etc/woops-agent",
		"USERPROFILE=C:\\Windows\\System32\\config\\systemprofile",
	}, home)
	got := map[string]string{}
	for _, e := range env {
		k, v, ok := splitEnv(e)
		if !ok {
			t.Fatalf("bad env entry %q", e)
		}
		got[k] = v
	}
	if got["PWD"] != home || got["HOME"] != home {
		t.Fatalf("HOME/PWD not forced: %#v", got)
	}
	if got["PATH"] == "" {
		t.Fatal("PATH dropped")
	}
	if runtime.GOOS == "windows" && got["USERPROFILE"] != home {
		t.Fatalf("USERPROFILE=%q want %q", got["USERPROFILE"], home)
	}
}

func TestIsWindowsSystemProfile(t *testing.T) {
	p := `C:\Windows\System32\config\systemprofile`
	if !isWindowsSystemProfile(p) {
		t.Fatalf("expected system profile: %s", p)
	}
	if isWindowsSystemProfile(`C:\Users\Administrator`) {
		t.Fatal("Administrator must not be treated as system profile")
	}
}

func TestWindowsProfileCandidatesPreferAdministrator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only candidate order")
	}
	cands := windowsProfileCandidates(`C:\Windows\System32\config\systemprofile`)
	if len(cands) == 0 || filepath.Base(cands[0]) != "Administrator" {
		t.Fatalf("want Administrator first, got %#v", cands)
	}
}

func TestWindowsProfileCandidatesScanUsers(t *testing.T) {
	users := t.TempDir()
	mustMkdir(t, filepath.Join(users, "Public"))
	mustMkdir(t, filepath.Join(users, "Default"))
	mustMkdir(t, filepath.Join(users, "ops-admin"))

	// Point candidate builder at temp Users via env SystemDrive + fake layout:
	// windowsUsersDir uses SystemDrive\Users — override by testing helpers
	// against add-order logic via isWindowsReservedUserDir + list.
	if isWindowsReservedUserDir("Public") != true || isWindowsReservedUserDir("ops-admin") {
		t.Fatal("reserved dir classification")
	}

	var names []string
	entries, err := os.ReadDir(users)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && !isWindowsReservedUserDir(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) != 1 || names[0] != "ops-admin" {
		t.Fatalf("scan filter: %#v", names)
	}
}

func TestDefaultShellHomeNeverSystemProfileOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	got := defaultShellHome()
	if isWindowsSystemProfile(got) {
		t.Fatalf("defaultShellHome returned system profile: %q", got)
	}
	if got == "" {
		t.Fatal("empty home")
	}
	if !dirExists(got) {
		t.Fatalf("home does not exist: %q", got)
	}
}

func TestWindowsInteractiveHomeSkipsMissingAdministrator(t *testing.T) {
	// When Administrator is missing, still return another scanned profile if present.
	// Cannot invent C:\Users on CI; just assert systemprofile alone never wins.
	got := windowsInteractiveHome(`C:\Windows\System32\config\systemprofile`)
	if got != "" && isWindowsSystemProfile(got) {
		t.Fatalf("must not return systemprofile: %q", got)
	}
	if got != "" && strings.EqualFold(filepath.Base(got), "Public") {
		t.Fatalf("Public is non-interactive fallback only via defaultShellHome, got %q", got)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func splitEnv(e string) (string, string, bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}
