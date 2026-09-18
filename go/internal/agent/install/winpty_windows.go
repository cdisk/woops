//go:build windows

package install

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/ops-bastion/ops/go/internal/tlsutil"
	"golang.org/x/sys/windows"
)

func requireElevated() error {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("admin check: %w", err)
	}
	defer token.Close()
	var elevated uint32
	var outLen uint32
	err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), uint32(unsafe.Sizeof(elevated)), &outLen)
	if err != nil {
		return fmt.Errorf("admin check: %w", err)
	}
	if elevated == 0 {
		return fmt.Errorf("administrator privileges required to install Woops Agent")
	}
	return nil
}

func windowsBuildNumber() int {
	mod := windows.NewLazySystemDLL("ntdll.dll")
	proc := mod.NewProc("RtlGetVersion")
	type osversioninfoex struct {
		OSVersionInfoSize uint32
		MajorVersion      uint32
		MinorVersion      uint32
		BuildNumber       uint32
		PlatformID        uint32
		CSDVersion        [128]uint16
	}
	var info osversioninfoex
	info.OSVersionInfoSize = uint32(unsafe.Sizeof(info))
	r1, _, _ := proc.Call(uintptr(unsafe.Pointer(&info)))
	if r1 != 0 {
		return 0
	}
	return int(info.BuildNumber)
}

func ensureWinpty(p Paths, gateway, code, pin string) error {
	build := windowsBuildNumber()
	if build == 0 || build >= 17763 {
		// ConPTY available; remove leftover winpty from older installs.
		_ = os.Remove(filepath.Join(p.BinDir, "winpty.dll"))
		_ = os.Remove(filepath.Join(p.BinDir, "winpty-agent.exe"))
		return nil
	}
	client, err := tlsutil.HTTPClient(tlsutil.DialOptions{Pin: pin}, 60*time.Second)
	if err != nil {
		return err
	}
	base := strings.TrimRight(gateway, "/") + "/i/" + code + "/winpty/amd64/"
	for _, name := range []string{"winpty.dll", "winpty-agent.exe"} {
		url := base + name
		dest := filepath.Join(p.BinDir, name)
		if err := downloadFile(client, url, dest); err != nil {
			return fmt.Errorf("winpty %s: %w", name, err)
		}
	}
	return nil
}

func downloadFile(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}