package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// recordingDir applies the guacd-specific requirement that the shared spool
// path be POSIX-absolute before allocating an operation directory.
func (h *Handler) recordingDir(operationID string) string {
	if h.deps.RecordingRoot == nil || h.deps.RecordingDir == nil ||
		!strings.HasPrefix(h.deps.RecordingRoot(), "/") {
		return ""
	}
	dir, err := h.deps.RecordingDir(operationID)
	if err != nil {
		log.Printf("ops-audit recording dir op=%s: %v", operationID, err)
		return ""
	}
	return dir
}

// recordingDetail discovers guacd's native output and returns replay metadata.
func recordingDetail(d Deps, dir string) map[string]any {
	if dir == "" || d.RecordingRelPath == nil {
		return nil
	}
	path := findRecording(dir)
	if path == "" {
		return nil
	}
	waitRecordingSettled(path)
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return nil
	}
	rel := d.RecordingRelPath(path)
	if rel == "" {
		return nil
	}
	sum, err := fileSHA256(path)
	if err != nil {
		log.Printf("ops-audit sha256 %s: %v", path, err)
		return nil
	}
	return map[string]any{
		"recordingPath": rel,
		"format":        "guacamole",
		"sha256":        sum,
		"size":          st.Size(),
	}
}

func findRecording(dir string) string {
	for _, name := range []string{guacRecordingName, guacRecordingName + ".guac"} {
		path := filepath.Join(dir, name)
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() > 0 {
			return path
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best, bestSize := "", int64(0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".cast") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Size() > bestSize {
			best, bestSize = filepath.Join(dir, entry.Name()), info.Size()
		}
	}
	return best
}

func waitRecordingSettled(path string) {
	last := int64(-1)
	for i := 0; i < 10; i++ {
		st, err := os.Stat(path)
		if err != nil {
			return
		}
		if st.Size() == last {
			return
		}
		last = st.Size()
		time.Sleep(100 * time.Millisecond)
	}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
