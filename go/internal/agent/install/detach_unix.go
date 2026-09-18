//go:build !windows

package install

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// DetachRestart launches a new session that survives parent death.
func DetachRestart(helperExe string, args []string) error {
	cmd := exec.Command(helperExe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("detach restart: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

// SelfExe returns the absolute path of the current executable.
func SelfExe() (string, error) {
	return os.Executable()
}
