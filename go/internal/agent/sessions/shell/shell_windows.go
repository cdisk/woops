//go:build windows

package shell

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"
	winpty "github.com/iamacarpet/go-winpty"
)

func defaultKind() string { return "powershell" }

func systemRoot() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}
	return `C:\Windows`
}

func powershellPath() string {
	return filepath.Join(systemRoot(), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

func cmdPath() string {
	return filepath.Join(systemRoot(), "System32", "cmd.exe")
}

// UTF-8 for xterm: set console encodings + chcp 65001 at session start.
const psUTF8Init = `[Console]::InputEncoding=[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false); chcp 65001 | Out-Null`

// effectiveShellKind maps powershell to cmd when ConPTY is unavailable (Win7 / Server 2012).
func effectiveShellKind(requested string) string {
	kind := strings.TrimSpace(strings.ToLower(requested))
	if kind == "" {
		kind = "powershell"
	}
	if !conpty.IsConPtyAvailable() && (kind == "powershell" || kind == "cmd") {
		return "cmd"
	}
	return kind
}

func buildWindowsCmdLine(kind, home string) (cmdline string, logKind string) {
	switch kind {
	case "cmd":
		// WorkDir/home is applied via ConPTY/WinPTY options; keep cmdline minimal for WinPTY.
		return cmdPath() + ` /K chcp 65001 >nul`, "cmd"
	case "powershell", "":
		homeInit := psUTF8Init
		if home != "" {
			homeInit += `; Set-Location -LiteralPath '` + strings.ReplaceAll(home, "'", "''") + `'`
		}
		return powershellPath() + ` -NoLogo -NoProfile -NoExit -Command "` + homeInit + `"`, "powershell"
	default:
		return "", kind
	}
}

func start(kind string, cols, rows int) (Session, error) {
	rawKind := kind
	kind = effectiveShellKind(kind)
	switch kind {
	case "powershell", "cmd":
		// ok
	case "bash":
		return nil, fmt.Errorf("bash is only available on Linux/macOS")
	default:
		return nil, fmt.Errorf("unsupported shell kind: %s", kind)
	}
	if rawKind == "powershell" && kind == "cmd" {
		log.Printf("shell: ConPTY unavailable; falling back powershell -> cmd")
	}

	home := defaultShellHome()
	cmdline, logKind := buildWindowsCmdLine(kind, home)
	if cmdline == "" {
		return nil, fmt.Errorf("unsupported shell kind: %s", kind)
	}
	env := envWithHome(os.Environ(), home)

	if conpty.IsConPtyAvailable() {
		opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(cols, rows), conpty.ConPtyEnv(env)}
		if home != "" {
			opts = append(opts, conpty.ConPtyWorkDir(home))
		}
		c, err := conpty.Start(cmdline, opts...)
		if err != nil {
			return nil, fmt.Errorf("ConPTY start failed: %w", err)
		}
		log.Printf("shell: ConPTY started kind=%s utf8 size=%dx%d dir=%s", logKind, cols, rows, home)
		return &conptySession{c: c}, nil
	}

	log.Printf("shell: ConPTY unavailable; using WinPTY for %s", logKind)
	return startWinPTY(cmdline, home, env, cols, rows, logKind)
}

type conptySession struct {
	c    *conpty.ConPty
	once sync.Once
}

func (s *conptySession) Read(p []byte) (int, error)  { return s.c.Read(p) }
func (s *conptySession) Write(p []byte) (int, error) { return s.c.Write(p) }

func (s *conptySession) Resize(cols, rows int) error {
	return s.c.Resize(cols, rows)
}

func (s *conptySession) Close() error {
	var err error
	s.once.Do(func() { err = s.c.Close() })
	return err
}

func winptyRuntimeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	for _, name := range []string{"winpty.dll", "winpty-agent.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return "", fmt.Errorf("%s missing next to woops-agent (%s); re-run: woops-agent install … (WinPTY required when ConPTY is unavailable)", name, dir)
		}
	}
	return dir, nil
}

func startWinPTY(cmdline, home string, env []string, cols, rows int, logKind string) (Session, error) {
	dllDir, err := winptyRuntimeDir()
	if err != nil {
		return nil, err
	}
	workDir := home
	if workDir == "" {
		workDir = dllDir
	}
	wp, err := winpty.OpenWithOptions(winpty.Options{
		DLLPrefix:   dllDir,
		Command:     cmdline,
		Dir:         workDir,
		Env:         env,
		InitialCols: uint32(cols),
		InitialRows: uint32(rows),
	})
	if err != nil {
		return nil, fmt.Errorf("WinPTY start failed: %w", err)
	}
	log.Printf("shell: WinPTY started kind=%s utf8 size=%dx%d dir=%s", logKind, cols, rows, workDir)
	return &winptySession{wp: wp}, nil
}

type winptySession struct {
	wp   *winpty.WinPTY
	once sync.Once
}

func (s *winptySession) Read(p []byte) (int, error)  { return s.wp.StdOut.Read(p) }
func (s *winptySession) Write(p []byte) (int, error) { return s.wp.StdIn.Write(p) }

func (s *winptySession) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	s.wp.SetSize(uint32(cols), uint32(rows))
	return nil
}

func (s *winptySession) Close() error {
	s.once.Do(func() { s.wp.Close() })
	return nil
}
