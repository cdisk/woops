package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Windows: rename fails if dest exists; remove then rename.
		_ = os.Remove(path)
		if err2 := os.Rename(tmpName, path); err2 != nil {
			return fmt.Errorf("atomic replace %s: %w", path, err2)
		}
	}
	cleanup = false
	return nil
}
