package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"os"
	"sync"
	"time"
	"unicode/utf8"
)

// Terminal geometry written into the cast header. The browser reports its real
// size right after connecting, which is recorded as a resize event.
const (
	castWidth  = 120
	castHeight = 40
)

// CastRecorder writes an asciinema cast v2 recording: one JSON header line
// followed by one JSON array per event. Frames are appended without fsync -
// losing a tail on a hard crash is preferable to slowing down a live session.
//
// Format: https://docs.asciinema.org/manual/asciicast/v2/
type CastRecorder struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	digest hash.Hash
	start  time.Time
	size   int64
	// pending holds a trailing incomplete UTF-8 sequence. A WebSocket frame can
	// split a multi-byte rune, and JSON string escaping would turn each half
	// into U+FFFD.
	pending []byte
	err     error
	closed  bool
	sum     string
}

type castHeader struct {
	Version   int    `json:"version"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Title     string `json:"title,omitempty"`
}

// NewCastRecorder creates path (truncating an existing file) and writes the header.
func NewCastRecorder(path string, title string) (*CastRecorder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	r := &CastRecorder{file: f, path: path, digest: sha256.New(), start: now}
	line, err := json.Marshal(castHeader{
		Version:   2,
		Width:     castWidth,
		Height:    castHeight,
		Timestamp: now.Unix(),
		Title:     title,
	})
	if err == nil {
		err = r.writeLineLocked(line)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return r, nil
}

// Path is the cast file location.
func (r *CastRecorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// WriteOutput records bytes travelling agent → browser (terminal output).
func (r *CastRecorder) WriteOutput(p []byte) {
	if r == nil || len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	buf := p
	if len(r.pending) > 0 {
		buf = make([]byte, 0, len(r.pending)+len(p))
		buf = append(buf, r.pending...)
		buf = append(buf, p...)
	}
	if cut := completeUTF8Len(buf); cut < len(buf) {
		r.pending = append(r.pending[:0], buf[cut:]...)
		buf = buf[:cut]
	} else {
		r.pending = r.pending[:0]
	}
	if len(buf) > 0 {
		r.eventLocked("o", string(buf))
	}
}

// WriteInput records bytes travelling browser → agent (keystrokes).
func (r *CastRecorder) WriteInput(p []byte) {
	if r == nil || len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.eventLocked("i", string(p))
}

// WriteResize records the terminal geometry the browser asked for.
func (r *CastRecorder) WriteResize(cols, rows int) {
	if r == nil || cols <= 0 || rows <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.eventLocked("r", fmt.Sprintf("%dx%d", cols, rows))
}

// Close flushes the recording and returns its SHA-256 (hex) and byte size for
// the operation's END detail. Safe to call twice; the second call repeats the
// first result.
func (r *CastRecorder) Close() (string, int64, error) {
	if r == nil {
		return "", 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.sum, r.size, r.err
	}
	if len(r.pending) > 0 {
		// The trailing bytes never completed a rune; record them anyway rather
		// than silently dropping output.
		r.eventLocked("o", string(r.pending))
		r.pending = nil
	}
	r.closed = true
	if err := r.file.Close(); err != nil && r.err == nil {
		r.err = err
	}
	r.sum = hex.EncodeToString(r.digest.Sum(nil))
	return r.sum, r.size, r.err
}

func (r *CastRecorder) eventLocked(kind, data string) {
	// Microsecond resolution matches what asciinema itself records.
	elapsed := math.Round(time.Since(r.start).Seconds()*1e6) / 1e6
	line, err := json.Marshal([]any{elapsed, kind, data})
	if err == nil {
		err = r.writeLineLocked(line)
	}
	if err != nil && r.err == nil {
		r.err = err
	}
}

func (r *CastRecorder) writeLineLocked(line []byte) error {
	line = append(line, '\n')
	n, err := r.file.Write(line)
	if n > 0 {
		r.size += int64(n)
		_, _ = r.digest.Write(line[:n])
	}
	return err
}

// completeUTF8Len returns the length of p truncated to the last complete UTF-8
// sequence, or len(p) when p already ends on a rune boundary.
func completeUTF8Len(p []byte) int {
	for back := 1; back < utf8.UTFMax && back <= len(p); back++ {
		b := p[len(p)-back]
		if b < utf8.RuneSelf {
			return len(p)
		}
		if b >= 0xC0 {
			need := 2
			switch {
			case b >= 0xF0:
				need = 4
			case b >= 0xE0:
				need = 3
			}
			if back >= need {
				return len(p)
			}
			return len(p) - back
		}
	}
	return len(p)
}
