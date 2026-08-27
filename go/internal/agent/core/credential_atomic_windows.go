//go:build windows

package core

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func replaceCredentialFile(tmp, dest string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return fmt.Errorf("invalid temporary credential path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return fmt.Errorf("invalid credential path: %w", err)
	}
	delay := 50 * time.Millisecond
	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	for attempt := 0; ; attempt++ {
		err = windows.MoveFileEx(from, to, flags)
		if err == nil {
			return nil
		}
		if attempt >= 5 || !credentialReplaceTransient(err) {
			return fmt.Errorf("replace credential file: %w", err)
		}
		time.Sleep(delay)
		delay *= 2
	}
}

func credentialReplaceTransient(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_BUSY)
}

func syncCredentialDir(string) error {
	// MoveFileEx with WRITE_THROUGH provides the Windows durability boundary.
	return nil
}
