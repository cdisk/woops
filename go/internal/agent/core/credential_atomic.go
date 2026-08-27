package core

import (
	"fmt"
	"os"
	"path/filepath"
)

func atomicWriteCredential(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if err = tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = replaceCredentialFile(tmpPath, path); err != nil {
		return err
	}
	if err = syncCredentialDir(dir); err != nil {
		return fmt.Errorf("sync credential directory: %w", err)
	}
	return nil
}
