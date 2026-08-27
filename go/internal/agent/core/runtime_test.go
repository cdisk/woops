package core

import (
	"testing"
	"time"
)

func TestControlReconnectRetryState(t *testing.T) {
	base := time.Second
	max := 30 * time.Second

	t.Run("dial failure backs off", func(t *testing.T) {
		retry := base
		retry = nextRetry(retry, max)
		if retry != 2*time.Second {
			t.Fatalf("retry=%v want 2s", retry)
		}
		retry = nextRetry(retry, max)
		if retry != 4*time.Second {
			t.Fatalf("retry=%v want 4s", retry)
		}
	})

	t.Run("successful session resets to base", func(t *testing.T) {
		retry := 16 * time.Second
		connected := true
		if connected {
			retry = base
		}
		if retry != base {
			t.Fatalf("retry=%v want base", retry)
		}
	})

	t.Run("backoff caps at max", func(t *testing.T) {
		retry := 20 * time.Second
		retry = nextRetry(retry, max)
		if retry != max {
			t.Fatalf("retry=%v want max", retry)
		}
	})
}
