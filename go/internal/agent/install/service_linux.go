//go:build !windows

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const unitPath = "/etc/systemd/system/woops-agent.service"

// EnsureService writes/refreshes the systemd unit (or no-ops preparation for nohup).
func EnsureService(p Paths) error {
	if !hasSystemd() {
		return nil
	}
	unit := fmt.Sprintf(`[Unit]
Description=Woops Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=OPS_AGENT_CONFIG=%s
ExecStart=%s -config %s
# Do not use ConfDir as WorkingDirectory: interactive shells override to home in Agent.
WorkingDirectory=/
Restart=always
RestartSec=3
KillMode=process
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`, p.Config, p.Bin, p.Config)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", serviceName).Run()
	return nil
}

// StopService stops systemd unit and kills leftover processes by exact name.
func StopService() error {
	if hasSystemd() {
		_ = exec.Command("systemctl", "stop", serviceName).Run()
	}
	_ = exec.Command("pkill", "-9", "-x", "woops-agent").Run()
	time.Sleep(time.Second)
	return nil
}

// StartService starts via systemd or nohup fallback.
func StartService() error {
	p := DefaultPaths()
	if hasSystemd() {
		if err := EnsureService(p); err != nil {
			return err
		}
		return exec.Command("systemctl", "restart", serviceName).Run()
	}
	cmd := exec.Command(p.Bin, "-config", p.Config)
	cmd.Env = append(os.Environ(), "OPS_AGENT_CONFIG="+p.Config)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// RemoveService disables and removes the systemd unit.
func RemoveService() error {
	_ = StopService()
	if hasSystemd() {
		_ = exec.Command("systemctl", "disable", serviceName).Run()
		_ = os.Remove(unitPath)
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	return nil
}

// AgentLive reports whether agent is running via systemd or process name.
func AgentLive() bool {
	if hasSystemd() {
		out, err := exec.Command("systemctl", "is-active", serviceName).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return true
		}
	}
	if err := exec.Command("pgrep", "-x", "woops-agent").Run(); err == nil {
		return true
	}
	return false
}

func hasSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	st, err := os.Stat("/run/systemd/system")
	return err == nil && st.IsDir()
}

// PreferLocalBin prefers /usr/local/bin; falls back to /opt when live and local missing.
func PreferLocalBin(live bool) Paths {
	p := DefaultPaths()
	if live {
		if _, err := os.Stat(p.Bin); err != nil {
			opt := "/opt/woops-agent/woops-agent"
			_ = os.MkdirAll(filepath.Dir(opt), 0o755)
			p.BinDir = filepath.Dir(opt)
			p.Bin = opt
			p.BinOld = opt + ".old"
		}
	}
	return p
}
