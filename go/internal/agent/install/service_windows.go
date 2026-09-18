//go:build windows

package install

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// EnsureService creates or updates the Windows service ImagePath and sets Automatic start.
func EnsureService(p Paths) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()

	binPath := `"` + p.Bin + `" -config "` + p.Config + `"`
	s, err := m.OpenService(serviceName)
	if err != nil {
		s, err = m.CreateService(serviceName, p.Bin, mgr.Config{
			DisplayName: "Woops Agent",
			Description: "Woops host agent",
			StartType:   mgr.StartAutomatic,
		}, "-config", p.Config)
		if err != nil {
			return fmt.Errorf("create service: %w", err)
		}
		s.Close()
		return nil
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	cfg.BinaryPathName = binPath
	cfg.StartType = mgr.StartAutomatic
	cfg.DisplayName = "Woops Agent"
	cfg.Description = "Woops host agent"
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	return nil
}

// StopService stops the Windows service and any leftover woops-agent processes.
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		killAgentProcesses()
		return nil
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		killAgentProcesses()
		return nil
	}
	defer s.Close()
	status, err := s.Control(svc.Stop)
	if err == nil {
		deadline := time.Now().Add(20 * time.Second)
		for status.State != svc.Stopped && time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			status, err = s.Query()
			if err != nil {
				break
			}
		}
	}
	killAgentProcesses()
	return nil
}

// StartService starts the Windows service.
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

// RemoveService stops and deletes the Windows service.
func RemoveService() error {
	_ = StopService()
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return nil
	}
	defer s.Close()
	return s.Delete()
}

// AgentLive reports whether the agent service or process is currently running.
func AgentLive() bool {
	m, err := mgr.Connect()
	if err == nil {
		defer m.Disconnect()
		if s, err := m.OpenService(serviceName); err == nil {
			defer s.Close()
			if st, err := s.Query(); err == nil && st.State == svc.Running {
				return true
			}
		}
	}
	return processNamedRunning("woops-agent.exe")
}

func killAgentProcesses() {
	_ = exec.Command("taskkill", "/F", "/IM", "woops-agent.exe").Run()
}

func processNamedRunning(exe string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return false
	}
	for {
		name := syscall.UTF16ToString(pe.ExeFile[:])
		if equalFoldASCII(name, exe) {
			return true
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			return false
		}
	}
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
