//go:build unix

package filemanager

import (
	"os"
	"path/filepath"
	"time"
)

// listRoots returns top-level directories under "/" for the explorer tree
// (analogous to Windows drive letters).
func listRoots() ([]entry, error) {
	infos, err := os.ReadDir("/")
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(infos))
	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		name := info.Name()
		full := filepath.Join("/", name)
		e := entry{Name: name, Path: full, IsDir: true}
		if fi, err := info.Info(); err == nil {
			e.Size = fi.Size()
			e.Mtime = fi.ModTime().Unix()
		} else {
			e.Mtime = time.Now().Unix()
		}
		out = append(out, e)
	}
	// Always include an explicit "/" node so users can open the root listing.
	out = append([]entry{{
		Name:  "/",
		Path:  "/",
		IsDir: true,
	}}, out...)
	return out, nil
}
