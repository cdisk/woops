//go:build unix

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func defaultKind() string { return "bash" }

func resolveUnixShell() (string, error) {
	// Prefer PATH (normal /usr/bin:/bin). Fall back to fixed paths when the
	// agent service has a sparse PATH and LookPath would miss a real shell.
	if p, err := exec.LookPath("bash"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("sh"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("bash/sh not found in PATH or /bin|/usr/bin")
}

func start(kind string, cols, rows int) (Session, error) {
	var cmd *exec.Cmd
	var shellPath string
	switch kind {
	case "bash":
		// Product kind stays "bash"; resolve a real binary (may be sh on minimal images).
		// Do not inherit the agent service's SHELL (often unset → dircolors errors).
		var err error
		shellPath, err = resolveUnixShell()
		if err != nil {
			return nil, err
		}
		cmd = exec.Command(shellPath, "-l")
	case "powershell":
		return nil, fmt.Errorf("powershell is only available on Windows")
	default:
		return nil, fmt.Errorf("unsupported shell kind: %s", kind)
	}
	home := defaultShellHome()
	// The agent commonly runs as a system service without a useful TERM.
	// The browser terminal is xterm.js, so advertise the matching terminal
	// capabilities to full-screen programs such as nano, vim and top.
	cmd.Env = append(envWithHome(os.Environ(), home),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"SHELL="+shellPath,
	)
	// Force interactive shell cwd to the user home (not the service WorkingDirectory).
	if home != "" {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixSession{file: f, cmd: cmd}, nil
}

type unixSession struct {
	file *os.File
	cmd  *exec.Cmd
}

func (s *unixSession) Read(p []byte) (int, error)  { return s.file.Read(p) }
func (s *unixSession) Write(p []byte) (int, error) { return s.file.Write(p) }

func (s *unixSession) Resize(cols, rows int) error {
	return pty.Setsize(s.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *unixSession) Close() error {
	_ = s.file.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGHUP)
		_, _ = s.cmd.Process.Wait()
	}
	return nil
}
