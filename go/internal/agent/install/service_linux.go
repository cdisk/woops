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

const (
	unitPath   = "/etc/systemd/system/woops-agent.service"
	sysvPath   = "/etc/init.d/woops-agent"
	sysvLevels = "3"
)

// EnsureService registers the agent for boot: systemd unit, or SysV init on
// CentOS 6 / hosts without systemd.
func EnsureService(p Paths) error {
	if hasSystemd() {
		return writeSystemdUnit(p)
	}
	return writeSysVInit(p)
}

func writeSystemdUnit(p Paths) error {
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

func writeSysVInit(p Paths) error {
	script := fmt.Sprintf(`#!/bin/bash
# chkconfig: 2345 90 10
# description: Woops Agent
### BEGIN INIT INFO
# Provides:          woops-agent
# Required-Start:    $network $remote_fs
# Required-Stop:     $network $remote_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Woops Agent
### END INIT INFO

BIN="%s"
CFG="%s"
PIDFILE=/var/run/woops-agent.pid
LOGDIR=/var/log/woops-agent

start() {
  if pgrep -x woops-agent >/dev/null 2>&1; then
    echo "woops-agent already running"
    return 0
  fi
  mkdir -p "$LOGDIR"
  export OPS_AGENT_CONFIG="$CFG"
  # setsid detaches from the calling shell (CentOS 6 has no systemd-run).
  if command -v setsid >/dev/null 2>&1; then
    setsid "$BIN" -config "$CFG" >>"$LOGDIR/woops-agent.log" 2>&1 &
  else
    nohup "$BIN" -config "$CFG" >>"$LOGDIR/woops-agent.log" 2>&1 &
  fi
  echo $! >"$PIDFILE"
  echo "woops-agent started"
}

stop() {
  pkill -9 -x woops-agent 2>/dev/null || true
  rm -f "$PIDFILE"
  echo "woops-agent stopped"
}

status() {
  if pgrep -x woops-agent >/dev/null 2>&1; then
    echo "woops-agent is running"
    return 0
  fi
  echo "woops-agent is stopped"
  return 3
}

case "$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; sleep 1; start ;;
  status) status ;;
  *) echo "Usage: $0 {start|stop|restart|status}"; exit 1 ;;
esac
`, p.Bin, p.Config)
	if err := os.WriteFile(sysvPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write SysV init: %w", err)
	}
	// CentOS/RHEL chkconfig; Debian update-rc.d when present.
	if _, err := exec.LookPath("chkconfig"); err == nil {
		_ = exec.Command("chkconfig", "--add", serviceName).Run()
		_ = exec.Command("chkconfig", serviceName, "on").Run()
	} else if _, err := exec.LookPath("update-rc.d"); err == nil {
		_ = exec.Command("update-rc.d", serviceName, "defaults").Run()
	}
	_ = sysvLevels
	return nil
}

// StopService stops systemd/SysV unit and kills leftover processes by exact name.
func StopService() error {
	if hasSystemd() {
		_ = exec.Command("systemctl", "stop", serviceName).Run()
	} else if _, err := os.Stat(sysvPath); err == nil {
		_ = exec.Command("service", serviceName, "stop").Run()
		_ = exec.Command(sysvPath, "stop").Run()
	}
	_ = exec.Command("pkill", "-9", "-x", "woops-agent").Run()
	time.Sleep(time.Second)
	return nil
}

// StartService starts via systemd, SysV, or a detached process fallback.
func StartService() error {
	p := resolveInstalledPaths()
	if err := EnsureService(p); err != nil {
		return err
	}
	if hasSystemd() {
		return exec.Command("systemctl", "restart", serviceName).Run()
	}
	if _, err := os.Stat(sysvPath); err == nil {
		if err := exec.Command(sysvPath, "restart").Run(); err == nil {
			return nil
		}
		if err := exec.Command("service", serviceName, "restart").Run(); err == nil {
			return nil
		}
	}
	return startDetached(p)
}

func startDetached(p Paths) error {
	_ = os.MkdirAll("/var/log/woops-agent", 0o750)
	cmd := exec.Command(p.Bin, "-config", p.Config)
	cmd.Env = append(os.Environ(), "OPS_AGENT_CONFIG="+p.Config)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	logFile, err := os.OpenFile("/var/log/woops-agent/woops-agent.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	return cmd.Start()
}

// RemoveService disables and removes systemd unit / SysV script.
func RemoveService() error {
	_ = StopService()
	if hasSystemd() {
		_ = exec.Command("systemctl", "disable", serviceName).Run()
		_ = os.Remove(unitPath)
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	if _, err := os.Stat(sysvPath); err == nil {
		if _, err := exec.LookPath("chkconfig"); err == nil {
			_ = exec.Command("chkconfig", serviceName, "off").Run()
			_ = exec.Command("chkconfig", "--del", serviceName).Run()
		} else if _, err := exec.LookPath("update-rc.d"); err == nil {
			_ = exec.Command("update-rc.d", "-f", serviceName, "remove").Run()
		}
		_ = os.Remove(sysvPath)
	}
	return nil
}

// AgentLive reports whether a *resident* agent is running (not this installer).
func AgentLive() bool {
	self := os.Getpid()
	if hasSystemd() {
		out, err := exec.Command("systemctl", "is-active", serviceName).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return true
		}
	}
	return otherAgentProcessRunning(self)
}

// otherAgentProcessRunning is true when a woops-agent process exists other than
// excludePID (the installer itself). Matching the installer used to flip LIVE=1
// on first install, which staged to /opt and then StartService looked for
// /usr/local/bin — silent start failure on every fresh Linux host.
func otherAgentProcessRunning(excludePID int) bool {
	out, err := exec.Command("pgrep", "-x", "woops-agent").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Fields(string(out)) {
		var pid int
		if _, err := fmt.Sscanf(line, "%d", &pid); err != nil {
			continue
		}
		if pid > 0 && pid != excludePID {
			return true
		}
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

// PreferLocalBin always prefers /usr/local/bin. The /opt fallback is only when
// that path already holds a live binary (ETXTBSY upgrades).
func PreferLocalBin(live bool) Paths {
	p := DefaultPaths()
	if !live {
		return p
	}
	if _, err := os.Stat(p.Bin); err == nil {
		return p
	}
	opt := "/opt/woops-agent/woops-agent"
	if _, err := os.Stat(opt); err == nil {
		p.BinDir = filepath.Dir(opt)
		p.Bin = opt
		p.BinOld = opt + ".old"
	}
	return p
}

// resolveInstalledPaths picks the binary that was actually installed.
func resolveInstalledPaths() Paths {
	p := DefaultPaths()
	if _, err := os.Stat(p.Bin); err == nil {
		return p
	}
	opt := "/opt/woops-agent/woops-agent"
	if _, err := os.Stat(opt); err == nil {
		p.BinDir = filepath.Dir(opt)
		p.Bin = opt
		p.BinOld = opt + ".old"
		return p
	}
	return p
}
