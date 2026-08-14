package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxSizeMB  = 50
	maxAgeDays = 90
	// MaxBackups=0: do not prune by count; MaxAge alone caps retention (~90d).
	maxBackups = 0
)

// dirOverride, when non-empty, replaces ResolveDir (tests only).
var dirOverride string

// SetDirOverrideForTest redirects ResolveDir; empty clears. Tests only.
func SetDirOverrideForTest(dir string) {
	dirOverride = dir
}

// ResolveDir returns the directory for Agent local log files.
// Linux: /var/log/woops-agent. Windows: config directory (ProgramData\woops-agent).
func ResolveDir(cfgPath string) string {
	if dirOverride != "" {
		return dirOverride
	}
	if runtime.GOOS == "windows" {
		dir := filepath.Dir(cfgPath)
		if dir == "" || dir == "." {
			return "."
		}
		return dir
	}
	return "/var/log/woops-agent"
}

// New opens a rotating log file under ResolveDir(cfgPath) named filename
// (e.g. "woops-agent.log", "proxy.log"). Output also mirrors to stderr.
func New(cfgPath, filename string) (*log.Logger, io.Closer, error) {
	dir := ResolveDir(cfgPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, fmt.Errorf("applog mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, filename)
	rot := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSizeMB,
		MaxAge:     maxAgeDays,
		MaxBackups: maxBackups,
		Compress:   true,
		LocalTime:  true,
	}
	out := io.MultiWriter(rot, os.Stderr)
	l := log.New(out, "", log.LstdFlags)
	return l, rot, nil
}

// SetupDefault configures the standard library logger to write woops-agent.log
// (rotated) and stderr. Failures leave stderr as-is and return the error;
// callers should still start the agent.
func SetupDefault(cfgPath string) (io.Closer, error) {
	l, closer, err := New(cfgPath, "woops-agent.log")
	if err != nil {
		return nil, err
	}
	log.SetOutput(l.Writer())
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("")
	return closer, nil
}
