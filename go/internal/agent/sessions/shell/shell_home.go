package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// defaultShellHome returns the directory interactive shells should start in.
// Prefer a real user profile over the agent service working directory
// (/etc/woops-agent, ProgramData\woops-agent, …).
func defaultShellHome() string {
	home, _ := os.UserHomeDir()
	home = strings.TrimSpace(home)

	if runtime.GOOS == "windows" {
		if h := windowsInteractiveHome(home); h != "" {
			return h
		}
		// Never start in LocalSystem's profile even if UserHomeDir points there.
		if isWindowsSystemProfile(home) {
			if pub := windowsPublicProfile(); dirExists(pub) {
				return pub
			}
			drive := windowsSystemDrive()
			if dirExists(drive + `\`) {
				return drive + `\`
			}
			return ""
		}
	}

	if home != "" && dirExists(home) {
		return home
	}
	if runtime.GOOS != "windows" && dirExists("/root") {
		return "/root"
	}
	return home
}

// windowsInteractiveHome prefers an interactive profile when the agent runs as
// LocalSystem (UserHomeDir → …\systemprofile), which is not what operators expect.
func windowsInteractiveHome(passwdHome string) string {
	for _, c := range windowsProfileCandidates(passwdHome) {
		if dirExists(c) {
			return c
		}
	}
	return ""
}

func windowsSystemDrive() string {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	return drive
}

func windowsUsersDir() string {
	return filepath.Join(windowsSystemDrive()+string(os.PathSeparator), "Users")
}

func windowsPublicProfile() string {
	return filepath.Join(windowsUsersDir(), "Public")
}

func windowsProfileCandidates(passwdHome string) []string {
	users := windowsUsersDir()
	var out []string
	seen := map[string]bool{}

	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || isWindowsSystemProfile(p) || isWindowsNonInteractiveProfile(p) {
			return
		}
		key := strings.ToLower(filepath.Clean(p))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, p)
	}

	// Prefer Administrator when running as SYSTEM / missing useful home.
	if passwdHome == "" || isWindowsSystemProfile(passwdHome) {
		add(filepath.Join(users, "Administrator"))
	}

	if up := strings.TrimSpace(os.Getenv("USERPROFILE")); up != "" {
		add(up)
	}
	if passwdHome != "" {
		add(passwdHome)
	}

	// Scan C:\Users\* so renamed / never-logged-in-as-Administrator hosts still
	// land in a real interactive profile instead of systemprofile.
	if entries, err := os.ReadDir(users); err == nil {
		var others []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if isWindowsReservedUserDir(name) {
				continue
			}
			p := filepath.Join(users, name)
			if strings.EqualFold(name, "Administrator") {
				add(p)
				continue
			}
			others = append(others, p)
		}
		for _, p := range others {
			add(p)
		}
	}

	return out
}

func isWindowsReservedUserDir(name string) bool {
	switch strings.ToLower(name) {
	case "public", "default", "default user", "all users", "desktop.ini":
		return true
	default:
		return false
	}
}

func isWindowsNonInteractiveProfile(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	return isWindowsReservedUserDir(base) || isWindowsSystemProfile(p)
}

func isWindowsSystemProfile(p string) bool {
	lower := strings.ToLower(filepath.Clean(p))
	return strings.Contains(lower, `\windows\system32\config\systemprofile`) ||
		strings.Contains(lower, `/windows/system32/config/systemprofile`)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// envWithHome copies env, forcing HOME/USERPROFILE/PWD so login shells and
// prompts do not inherit the service WorkingDirectory via PWD.
// Empty or stale SHELL from the agent service is dropped so callers can set it.
func envWithHome(base []string, home string) []string {
	out := make([]string, 0, len(base)+5)
	for _, e := range base {
		switch {
		case strings.HasPrefix(e, "HOME="),
			strings.HasPrefix(e, "PWD="),
			strings.HasPrefix(e, "USERPROFILE="),
			strings.HasPrefix(e, "HOMEDRIVE="),
			strings.HasPrefix(e, "HOMEPATH="),
			strings.HasPrefix(e, "SHELL="):
			continue
		default:
			out = append(out, e)
		}
	}
	if home != "" {
		out = append(out, "HOME="+home, "PWD="+home)
		if runtime.GOOS == "windows" {
			out = append(out, "USERPROFILE="+home)
			vol := filepath.VolumeName(home)
			if vol != "" {
				out = append(out, "HOMEDRIVE="+vol)
				out = append(out, "HOMEPATH="+strings.TrimPrefix(home, vol))
			}
		}
	}
	return out
}
