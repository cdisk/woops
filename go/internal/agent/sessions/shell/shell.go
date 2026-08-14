package shell

import (
	"fmt"
	"io"
)

// Session is a local interactive shell with optional PTY resize.
type Session interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// Start launches a local shell by kind: bash (unix) | powershell (windows).
// cols/rows are the initial PTY geometry (must match the browser xterm).
func Start(kind string, cols, rows int) (Session, error) {
	cols, rows = normalizeSize(cols, rows)
	switch kind {
	case "bash", "powershell", "":
		if kind == "" {
			kind = defaultKind()
		}
		return start(kind, cols, rows)
	case "cmd":
		return nil, fmt.Errorf("cmd is not supported; use powershell")
	default:
		return nil, fmt.Errorf("unsupported shell kind: %s", kind)
	}
}
