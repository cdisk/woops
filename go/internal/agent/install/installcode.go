package install

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// WriteInstallCode atomically publishes the one-shot install code with
// restricted permissions (0600 on Unix; SYSTEM+Administrators on Windows).
func WriteInstallCode(path, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("empty install code")
	}
	dir := filepathDir(path)
	tmp, err := os.CreateTemp(dir, ".install-code.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(code + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpName, 0o600); err != nil {
			return err
		}
	}
	_ = os.Remove(path)
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0o600)
	}
	// ACL after rename: restricting the temp file first would lock out the
	// non-elevated installer process from renaming it on Windows.
	return restrictWindowsACL(path)
}

func filepathDir(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return "."
	}
	return path[:i]
}

func restrictWindowsACL(path string) error {
	cmd := exec.Command("icacls.exe", path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls %s: %w (%s)", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}
