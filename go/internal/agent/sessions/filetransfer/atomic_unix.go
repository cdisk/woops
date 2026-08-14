//go:build !windows

package filetransfer

import "os"

func replaceFile(tmp, dest string) error {
	return os.Rename(tmp, dest)
}
