package install

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// Uninstall stops and removes the service, deletes the binary, and optionally
// purges the config directory. Must not run from the install-path binary;
// re-exec from a temp copy when needed.
func Uninstall(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	logf := opts.Logger
	p := DefaultPaths()

	self, err := SelfExe()
	if err != nil {
		return err
	}
	self, _ = filepath.Abs(self)
	if sameFile(self, p.Bin) {
		logf.Printf("==> Re-exec uninstall from temp copy")
		return reexecUninstallFromTemp(self, opts)
	}

	logf.Printf("==> Stopping service")
	_ = StopService()
	logf.Printf("==> Removing service")
	_ = RemoveService()

	logf.Printf("==> Removing binary %s", p.Bin)
	_ = os.Remove(p.Bin)
	_ = os.Remove(p.BinOld)
	if runtime.GOOS == "windows" {
		_ = os.Remove(filepath.Join(p.BinDir, "winpty.dll"))
		_ = os.Remove(filepath.Join(p.BinDir, "winpty-agent.exe"))
		_ = os.Remove(filepath.Join(p.BinDir, "woops-agent-new.exe"))
	} else {
		_ = os.Remove(p.Bin + "-new")
	}

	if opts.Purge {
		logf.Printf("==> Purging %s", p.ConfDir)
		_ = os.RemoveAll(p.ConfDir)
	} else {
		_ = os.Remove(p.CodeFile)
		cleanupHelper(p)
	}
	logf.Printf("==> Uninstalled")
	return nil
}

func reexecUninstallFromTemp(self string, opts Options) error {
	tmpDir := os.TempDir()
	name := "woops-agent-uninstall"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	tmp := filepath.Join(tmpDir, name)
	if err := copyFile(self, tmp); err != nil {
		return fmt.Errorf("stage uninstall helper: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	args := []string{"uninstall"}
	if opts.Purge {
		args = append(args, "-purge")
	}
	// Run synchronously from temp so the operator sees the result.
	return runUninstallSync(tmp, args)
}
