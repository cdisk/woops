//go:build windows

package install

import (
	"os"
	"os/exec"
)

func runUninstallSync(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
