package wsutil

import (
	"testing"
	"time"
)

func TestPingConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"OPS_WS_PING_SECONDS":         "30",
		"OPS_WS_PING_SECONDS_SHELL":   "5",
		"OPS_WS_PING_SECONDS_FILE":    "0",
		"OPS_WS_PING_SECONDS_DESKTOP": "99",
		"OPS_WS_PING_SECONDS_EXEC":    "nope",
		"OPS_WS_PING_SECONDS_PORTMAP": "1",
	}
	cfg := FromEnv(func(k string) string { return env[k] })
	if got := cfg.For("control"); got != 30*time.Second {
		t.Fatalf("control %s", got)
	}
	if got := cfg.For("shell"); got != 5*time.Second {
		t.Fatalf("shell %s", got)
	}
	if got := cfg.For("file"); got != time.Second {
		t.Fatalf("file %s", got)
	}
	if got := cfg.For("desktop"); got != 60*time.Second {
		t.Fatalf("desktop %s", got)
	}
	if got := cfg.For("exec"); got != 30*time.Second {
		t.Fatalf("exec %s", got)
	}
	if got := cfg.For("portmap"); got != time.Second {
		t.Fatalf("portmap %s", got)
	}
	if got := cfg.For(""); got != 30*time.Second {
		t.Fatalf("empty kind %s", got)
	}
}

func TestPingConfigDefaults(t *testing.T) {
	cfg := FromEnv(func(string) string { return "" })
	if got := cfg.For("shell"); got != DefaultPing {
		t.Fatalf("default %s", got)
	}
	if got := (PingConfig{}).For("control"); got != DefaultPing {
		t.Fatalf("zero config %s", got)
	}
}

func TestIdleForPingBounds(t *testing.T) {
	cases := []struct {
		ping time.Duration
		want time.Duration
	}{
		{time.Second, 15 * time.Second},
		{5 * time.Second, 15 * time.Second},
		{25 * time.Second, 75 * time.Second},
		{60 * time.Second, 90 * time.Second},
		{0, 75 * time.Second},
	}
	for _, tc := range cases {
		if got := IdleForPing(tc.ping); got != tc.want {
			t.Fatalf("idle(%s)=%s want %s", tc.ping, got, tc.want)
		}
	}
}
