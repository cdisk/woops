package shell

import "fmt"

// CastTitle labels a shell recording for players that show one.
func CastTitle(shellKind, username, assetID string) string {
	kind := shellKind
	if kind == "" {
		kind = "shell"
	}
	if username == "" {
		return kind + " " + assetID
	}
	return kind + " " + username + "@" + assetID
}

// ParseResize reads the shell channel's "R,cols,rows" control frame.
func ParseResize(data []byte) (cols, rows int, ok bool) {
	if _, err := fmt.Sscanf(string(data), "R,%d,%d", &cols, &rows); err != nil {
		return 0, 0, false
	}
	return cols, rows, cols > 0 && rows > 0
}
