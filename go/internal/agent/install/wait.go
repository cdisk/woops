package install

import (
	"context"
	"os"
	"strings"
	"time"
)

// WaitRegistered polls until asset-id and agent-token are non-empty and
// install-code has been consumed (deleted) by Agent bootstrap.
func WaitRegistered(ctx context.Context, p Paths, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if registered(p) {
			return nil
		}
		if time.Now().After(deadline) {
			return errRegistrationTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func registered(p Paths) bool {
	id := strings.TrimSpace(readFile(p.AssetID))
	tok := strings.TrimSpace(readFile(p.Token))
	if id == "" || tok == "" {
		return false
	}
	_, err := os.Stat(p.CodeFile)
	return os.IsNotExist(err)
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

type timeoutError string

func (e timeoutError) Error() string { return string(e) }

const errRegistrationTimeout = timeoutError("agent registration did not complete within timeout")
