// Package opsaudit spools runtime activity envelopes to append-only JSONL
// segments on local disk. control-api tails the segments with a byte offset
// cursor, so Gateway never talks to PostgreSQL.
//
// Layout:
//
//	{dir}/{yyyy}/{mm}/{dd}/events-{instanceID}-{seq}.open   active segment
//	{dir}/{yyyy}/{mm}/{dd}/events-{instanceID}-{seq}.jsonl  sealed segment
//	{dir}/{yyyy}/{mm}/{dd}/recordings/{operationId}/...     session recordings
//	{dir}/.cursor/{segmentBasename}.offset                  ingest cursor (control-api)
package audit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SegmentMaxBytes rotates the active segment so ingest never has to hold a
// huge file open and so retention can drop whole files.
const SegmentMaxBytes int64 = 64 << 20

// Writer appends envelopes to the current segment. Safe for concurrent use.
type Writer struct {
	dir        string
	instanceID string

	mu      sync.Mutex
	file    *os.File
	path    string // active .open path
	day     string // yyyy-mm-dd of the active segment
	seq     int
	written int64
}

// NewWriter prepares the spool directory and opens the first segment.
func NewWriter(dir, instanceID string) (*Writer, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("opsaudit: dir required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, ".cursor"), 0o750); err != nil {
		return nil, err
	}
	w := &Writer{dir: abs, instanceID: InstanceID(instanceID)}
	if err := w.rotateLocked(time.Now().UTC()); err != nil {
		return nil, err
	}
	return w, nil
}

// Dir is the spool root.
func (w *Writer) Dir() string { return w.dir }

// InstanceID resolves the segment owner id: explicit value, then
// OPS_GATEWAY_INSTANCE_ID, then hostname plus a short random suffix. The random
// suffix keeps segments distinct when a host restarts with the same name while
// an older segment is still being ingested.
func InstanceID(explicit string) string {
	if s := sanitize(explicit); s != "" {
		return s
	}
	if s := sanitize(os.Getenv("OPS_GATEWAY_INSTANCE_ID")); s != "" {
		return s
	}
	host, _ := os.Hostname()
	host = sanitize(host)
	if host == "" {
		host = "gateway"
	}
	if len(host) > 32 {
		host = host[:32]
	}
	return host + "-" + randomHex(3)
}

// Append writes one envelope as a single JSON line.
func (w *Writer) Append(envelope map[string]any) error {
	if w == nil {
		return nil
	}
	if len(envelope) == 0 {
		return fmt.Errorf("opsaudit: empty envelope")
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().UTC()
	if w.file == nil || w.day != dayKey(now) || w.written+int64(len(line)) > SegmentMaxBytes {
		if err := w.rotateLocked(now); err != nil {
			return err
		}
	}
	n, err := w.file.Write(line)
	w.written += int64(n)
	if err != nil {
		return err
	}
	// Ingest reads by byte offset, so the bytes must be visible promptly.
	return w.file.Sync()
}

// RecordingDir returns (and creates) the per-operation recording directory
// under today's date partition.
//
// Mode is world-writable: guacd runs as uid 1000 in Compose and shares this
// volume with Gateway (root). Date partitions get 0755 so guacd can traverse;
// a 0750 root-owned tree silently yields empty RDP/VNC recordings.
func (w *Writer) RecordingDir(operationID string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("opsaudit: writer disabled")
	}
	id := sanitize(operationID)
	if id == "" {
		return "", fmt.Errorf("opsaudit: operationId required")
	}
	path := filepath.Join(w.dayDir(time.Now().UTC()), "recordings", id)
	if err := os.MkdirAll(path, 0o777); err != nil {
		return "", err
	}
	if err := chmodSharedRecordingTree(w.dir, path); err != nil {
		return "", err
	}
	return path, nil
}

// chmodSharedRecordingTree makes leaf 0777 and ancestors 0755 (through root)
// so a non-root guacd can create session files Gateway will later checksum.
func chmodSharedRecordingTree(root, leaf string) error {
	root = filepath.Clean(root)
	for p := filepath.Clean(leaf); ; p = filepath.Dir(p) {
		mode := os.FileMode(0o755)
		if p == filepath.Clean(leaf) {
			mode = 0o777
		}
		if err := os.Chmod(p, mode); err != nil {
			return err
		}
		if p == root {
			return nil
		}
		next := filepath.Dir(p)
		if next == p {
			return nil
		}
	}
}

// RelPath expresses an absolute path inside the spool as a forward-slash path
// relative to the spool root. control-api resolves it against its own
// OPS_AUDIT_DIR mount, which is rarely the same absolute path.
func (w *Writer) RelPath(abs string) string {
	if w == nil {
		return ""
	}
	rel, err := filepath.Rel(w.dir, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Close seals the active segment so ingest can finish it and stop polling.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sealLocked()
}

func (w *Writer) dayDir(now time.Time) string {
	return filepath.Join(w.dir, now.Format("2006"), now.Format("01"), now.Format("02"))
}

// rotateLocked seals the active segment and opens the next one.
func (w *Writer) rotateLocked(now time.Time) error {
	if err := w.sealLocked(); err != nil {
		return err
	}
	// seq keeps climbing across day boundaries so one process never writes two
	// segments with the same basename in different date partitions.
	day := dayKey(now)
	dir := w.dayDir(now)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// Skip sequence numbers already on disk (Gateway restart within the same day).
	for {
		base := fmt.Sprintf("events-%s-%d", w.instanceID, w.seq)
		open := filepath.Join(dir, base+".open")
		sealed := filepath.Join(dir, base+".jsonl")
		if exists(open) || exists(sealed) {
			w.seq++
			continue
		}
		f, err := os.OpenFile(open, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
		w.file = f
		w.path = open
		w.day = day
		w.written = 0
		return nil
	}
}

func (w *Writer) sealLocked() error {
	if w.file == nil {
		return nil
	}
	f, path := w.file, w.path
	w.file, w.path, w.written = nil, "", 0
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(path, strings.TrimSuffix(path, ".open")+".jsonl")
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sanitize keeps path- and filename-safe characters only.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == '.' || r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:n*2]
	}
	return hex.EncodeToString(buf)
}
