package install

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// InstallBinary copies srcExe to dst (the final install path), keeping one
// generation of rollback as dst+".old". The installer itself must already be
// running from a temp path so dst is not locked by this process.
func InstallBinary(srcExe, dst, dstOld string) error {
	if srcExe == "" || dst == "" {
		return fmt.Errorf("binary paths required")
	}
	if err := os.MkdirAll(dirOf(dst), 0o755); err != nil {
		return err
	}
	// Drop previous .old so rename of current → .old succeeds.
	_ = os.Remove(dstOld)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, dstOld); err != nil {
			// Windows may refuse rename of a running image; try copy+remove.
			if copyErr := copyFile(dst, dstOld); copyErr != nil {
				return fmt.Errorf("backup running binary: rename=%v copy=%v", err, copyErr)
			}
			_ = os.Remove(dst)
		}
	}
	if err := copyFile(srcExe, dst); err != nil {
		// Best-effort restore.
		if _, e := os.Stat(dstOld); e == nil {
			_ = os.Rename(dstOld, dst)
		}
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dst, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// RestoreBinary swaps dstOld back onto dst (upgrade rollback).
func RestoreBinary(dst, dstOld string) error {
	if _, err := os.Stat(dstOld); err != nil {
		return fmt.Errorf("no rollback binary at %s", dstOld)
	}
	_ = os.Remove(dst)
	if err := os.Rename(dstOld, dst); err != nil {
		if copyErr := copyFile(dstOld, dst); copyErr != nil {
			return fmt.Errorf("restore: rename=%v copy=%v", err, copyErr)
		}
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(dst, 0o755)
	}
	return nil
}

// RemoveOldBinary deletes the rollback copy after a successful upgrade.
func RemoveOldBinary(dstOld string) {
	_ = os.Remove(dstOld)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".copying"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
