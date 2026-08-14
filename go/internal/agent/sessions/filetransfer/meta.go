package filetransfer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	tempPrefix = ".ops-upload."
	partSuffix = ".part"
	metaSuffix = ".meta"
)

// Meta is persisted beside the .part file so reconnects can resume.
type Meta struct {
	TransferID  string `json:"transferId"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Fingerprint string `json:"fingerprint"`
	Offset      int64  `json:"offset"`
	ContentSHA  string `json:"contentSha256,omitempty"` // hex of running SHA-256 over accepted bytes
}

func sanitizeBase(name string) string {
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(os.PathSeparator) {
		name = "file"
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "file"
	}
	return out
}

func tempPaths(finalPath, transferID string) (partPath, metaPath string) {
	dir := filepath.Dir(finalPath)
	base := sanitizeBase(finalPath)
	id := strings.TrimSpace(transferID)
	stem := tempPrefix + base + "." + id
	return filepath.Join(dir, stem+partSuffix), filepath.Join(dir, stem+metaSuffix)
}

func writeMetaAtomic(metaPath string, m Meta) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := metaPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, metaPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readMeta(metaPath string) (Meta, error) {
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func removeTemps(partPath, metaPath string) {
	_ = os.Remove(partPath)
	_ = os.Remove(metaPath)
	_ = os.Remove(metaPath + ".tmp")
}

func validateTransferID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("transferId required")
	}
	if len(id) > 64 {
		return fmt.Errorf("transferId too long")
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("invalid transferId")
	}
	return nil
}
