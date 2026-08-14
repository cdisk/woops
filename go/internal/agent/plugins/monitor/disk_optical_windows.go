//go:build windows

package monitor

import (
	"strings"

	"golang.org/x/sys/windows"
)

// isOpticalDrive reports whether mount is a CD/DVD drive letter.
// gopsutil includes DRIVE_CDROM in Partitions(false); empty or full discs
// often show ~100% used and should not feed capacity alerts.
func isOpticalDrive(mount string) bool {
	path := strings.TrimSpace(mount)
	if path == "" {
		return false
	}
	// Accept "D:", "D:\", "D:/" forms from gopsutil.
	if len(path) >= 2 && path[1] == ':' {
		path = path[:2] + `\`
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	return windows.GetDriveType(p) == windows.DRIVE_CDROM
}
