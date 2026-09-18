package install

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Options for install / uninstall.
type Options struct {
	Gateway string
	Code    string
	Pin     string
	Phase   string // "" | "restart"
	Purge   bool   // uninstall: also remove conf dir
	Timeout time.Duration
	Logger  *log.Logger
}

// Run is the entry for `woops-agent install`.
func Run(opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.Phase == "restart" {
		return runRestartPhase(opts)
	}
	return runInstall(opts)
}

func runInstall(opts Options) error {
	if opts.Gateway == "" || opts.Code == "" {
		return fmt.Errorf("install requires -gateway and -code")
	}
	if err := requireElevated(); err != nil {
		return err
	}
	live := AgentLive()
	p := PreferLocalBin(live)
	if err := p.EnsureDirs(); err != nil {
		return err
	}

	logf := opts.Logger
	logf.Printf("==> Install woops-agent (live=%v)", live)
	logf.Printf("==> Gateway: %s", opts.Gateway)

	self, err := SelfExe()
	if err != nil {
		return err
	}
	self, err = filepath.Abs(self)
	if err != nil {
		return err
	}

	// Config before install-code so a starting Agent never sees stale gateway + new code.
	if err := WriteAgentYAML(p.Config, opts.Gateway, opts.Pin); err != nil {
		return fmt.Errorf("write agent.yaml: %w", err)
	}
	logf.Printf("==> Wrote %s", p.Config)

	if err := WriteInstallCode(p.CodeFile, opts.Code); err != nil {
		return fmt.Errorf("write install-code: %w", err)
	}
	logf.Printf("==> Published install-code")

	// Copy self into install path only when we are not already that file.
	if !sameFile(self, p.Bin) {
		if err := InstallBinary(self, p.Bin, p.BinOld); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
		logf.Printf("==> Installed binary %s", p.Bin)
	} else {
		logf.Printf("==> Binary already at install path")
	}

	if err := ensureWinpty(p, opts.Gateway, opts.Code, opts.Pin); err != nil {
		return fmt.Errorf("winpty: %w", err)
	}

	if err := EnsureService(p); err != nil {
		return fmt.Errorf("ensure service: %w", err)
	}
	logf.Printf("==> Service registered")

	// Always detach restart so WebShell/exec sessions survive and Windows file locks clear.
	helper, err := stageHelper(self, p)
	if err != nil {
		return err
	}
	args := []string{
		"install",
		"-phase", "restart",
		"-gateway", opts.Gateway,
		"-code", opts.Code,
		"-pin", opts.Pin,
	}
	logf.Printf("==> Staging detached restart")
	if err := DetachRestart(helper, args); err != nil {
		return err
	}
	if live {
		logf.Printf("==> Update staged; service restart scheduled")
		return nil
	}
	logf.Printf("==> Waiting up to %s for Agent bootstrap registration...", opts.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout+5*time.Second)
	defer cancel()
	// Give the detached process a moment to stop/start.
	time.Sleep(3 * time.Second)
	if err := WaitRegistered(ctx, p, opts.Timeout); err != nil {
		return fmt.Errorf("%w; check %s and service status", err, filepath.Join(p.ConfDir, "woops-agent.log"))
	}
	RemoveOldBinary(p.BinOld)
	logf.Printf("==> Agent registered successfully")
	return nil
}

func runRestartPhase(opts Options) error {
	p := PreferLocalBin(true)
	_ = p.EnsureDirs()
	f, err := os.OpenFile(p.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		defer f.Close()
		opts.Logger = log.New(io.MultiWriter(os.Stderr, f), "", log.LstdFlags)
	}
	logf := opts.Logger
	logf.Printf("restart-phase begin")

	time.Sleep(2 * time.Second)
	_ = StopService()
	time.Sleep(2 * time.Second)

	// Promote any leftover staged copies from older script installers.
	_ = promoteLegacyStaged(p)

	if err := StartService(); err != nil {
		logf.Printf("start failed: %v; attempting rollback", err)
		if rb := RestoreBinary(p.Bin, p.BinOld); rb != nil {
			logf.Printf("rollback binary failed: %v", rb)
		} else {
			_ = StartService()
		}
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if err := WaitRegistered(ctx, p, opts.Timeout); err != nil {
		logf.Printf("registration timeout; rolling back: %v", err)
		_ = StopService()
		if rb := RestoreBinary(p.Bin, p.BinOld); rb != nil {
			logf.Printf("rollback binary failed: %v", rb)
			return err
		}
		_ = StartService()
		return fmt.Errorf("registration failed, rolled back: %w", err)
	}
	RemoveOldBinary(p.BinOld)
	cleanupHelper(p)
	logf.Printf("restart-phase ok")
	return nil
}

func stageHelper(self string, p Paths) (string, error) {
	helper := filepath.Join(p.ConfDir, ".woops-agent-install."+strconv.Itoa(os.Getpid()))
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if err := copyFile(self, helper); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(helper, 0o755)
	}
	return helper, nil
}

func cleanupHelper(p Paths) {
	matches, _ := filepath.Glob(filepath.Join(p.ConfDir, ".woops-agent-install.*"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

func promoteLegacyStaged(p Paths) error {
	candidates := []string{p.Bin + "-new", p.Bin + ".new"}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(p.BinDir, "woops-agent-new.exe"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			_ = os.Remove(p.BinOld)
			_ = os.Rename(p.Bin, p.BinOld)
			return os.Rename(c, p.Bin)
		}
	}
	return nil
}

func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return equalFoldASCII(aa, bb)
	}
	return aa == bb
}
