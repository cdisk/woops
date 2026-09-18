//go:build windows

package install

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// DetachRestart launches a detached copy of this binary to finish stop/start
// after the parent (often an exec/WebShell session) exits.
func DetachRestart(helperExe string, args []string) error {
	cmd := exec.Command(helperExe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("detach restart: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

// PreferLocalBin is a no-op on Windows (single install path under Program Files).
func PreferLocalBin(live bool) Paths {
	_ = live
	return DefaultPaths()
}

// SelfExe returns the absolute path of the current executable.
func SelfExe() (string, error) {
	return os.Executable()
}
