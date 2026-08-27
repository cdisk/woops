//go:build !windows

package core

import "os"

func replaceCredentialFile(tmp, dest string) error {
	return os.Rename(tmp, dest)
}

func syncCredentialDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
