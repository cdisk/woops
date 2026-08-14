//go:build windows

package filetransfer

import "os"

func replaceFile(tmp, dest string) error {
	// Windows Rename fails when dest exists.
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}
