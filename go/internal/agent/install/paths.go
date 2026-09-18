package install

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	serviceName = "woops-agent"
	binNameUnix = "woops-agent"
	binNameWin  = "woops-agent.exe"
)

// Paths holds install locations for the current OS.
type Paths struct {
	ConfDir  string
	BinDir   string
	Bin      string
	BinOld   string
	Config   string
	CodeFile string
	AssetID  string
	Token    string
	LogFile  string
}

// DefaultPaths returns the standard install layout.
func DefaultPaths() Paths {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		conf := filepath.Join(pd, "woops-agent")
		binDir := filepath.Join(pf, "woops-agent")
		bin := filepath.Join(binDir, binNameWin)
		return Paths{
			ConfDir:  conf,
			BinDir:   binDir,
			Bin:      bin,
			BinOld:   bin + ".old",
			Config:   filepath.Join(conf, "agent.yaml"),
			CodeFile: filepath.Join(conf, "install-code"),
			AssetID:  filepath.Join(conf, "asset-id"),
			Token:    filepath.Join(conf, "agent-token"),
			LogFile:  filepath.Join(conf, "install.log"),
		}
	}
	conf := "/etc/woops-agent"
	bin := "/usr/local/bin/" + binNameUnix
	return Paths{
		ConfDir:  conf,
		BinDir:   filepath.Dir(bin),
		Bin:      bin,
		BinOld:   bin + ".old",
		Config:   filepath.Join(conf, "agent.yaml"),
		CodeFile: filepath.Join(conf, "install-code"),
		AssetID:  filepath.Join(conf, "asset-id"),
		Token:    filepath.Join(conf, "agent-token"),
		LogFile:  filepath.Join(conf, "install.log"),
	}
}

// EnsureDirs creates conf and bin directories with appropriate permissions.
func (p Paths) EnsureDirs() error {
	if err := os.MkdirAll(p.ConfDir, 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(p.BinDir, 0o755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.MkdirAll("/var/log/woops-agent", 0o750)
	}
	return nil
}
