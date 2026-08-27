//go:build windows

package filetransfer

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

const replaceAttempts = 6

func replaceFile(tmp, dest string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return fmt.Errorf("invalid temporary path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}

	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	delay := 50 * time.Millisecond
	for attempt := 1; ; attempt++ {
		err = windows.MoveFileEx(from, to, flags)
		if err == nil {
			return nil
		}
		if attempt >= replaceAttempts || !isTransientReplaceError(err) {
			return fmt.Errorf("replace %q: %w", dest, err)
		}
		// Indexers and antivirus software can briefly hold the destination on
		// legacy Windows. Retry without deleting the original file first.
		time.Sleep(delay)
		delay *= 2
	}
}

func isTransientReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_BUSY)
}
