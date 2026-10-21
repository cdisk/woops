package wsutil

import (
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	PingMin = time.Second
	PingMax = 60 * time.Second
	// IdleMin is the shortest control read timeout derived from a ping interval.
	IdleMin = 15 * time.Second
)

// PingConfig is the Gateway WebSocket ping schedule.
// Global applies when a kind is unset. Kind keys are control, shell, file,
// exec, desktop, and portmap.
type PingConfig struct {
	Global time.Duration
	ByKind map[string]time.Duration
}

// DefaultPingConfig is 25s for every kind.
func DefaultPingConfig() PingConfig {
	return PingConfig{Global: DefaultPing, ByKind: map[string]time.Duration{}}
}

// FromEnv reads OPS_WS_PING_SECONDS and OPS_WS_PING_SECONDS_<KIND>.
// Empty values are skipped. Non-numeric values are ignored. Out-of-range
// values are clamped to 1..60 seconds.
func FromEnv(getenv func(string) string) PingConfig {
	if getenv == nil {
		return DefaultPingConfig()
	}
	cfg := DefaultPingConfig()
	if d, ok := parsePingSeconds(getenv("OPS_WS_PING_SECONDS"), "OPS_WS_PING_SECONDS"); ok {
		cfg.Global = d
	}
	for _, kind := range []string{"control", "shell", "file", "exec", "desktop", "portmap"} {
		key := "OPS_WS_PING_SECONDS_" + strings.ToUpper(kind)
		if d, ok := parsePingSeconds(getenv(key), key); ok {
			if cfg.ByKind == nil {
				cfg.ByKind = map[string]time.Duration{}
			}
			cfg.ByKind[kind] = d
		}
	}
	return cfg
}

// For returns the ping interval for kind, or the global default.
func (c PingConfig) For(kind string) time.Duration {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if c.ByKind != nil {
		if d, ok := c.ByKind[kind]; ok && d > 0 {
			return d
		}
	}
	if c.Global > 0 {
		return c.Global
	}
	return DefaultPing
}

// IdleForPing is the control read timeout: 3×ping, clamped to 15..90 seconds.
func IdleForPing(ping time.Duration) time.Duration {
	if ping <= 0 {
		ping = DefaultPing
	}
	idle := ping * 3
	if idle < IdleMin {
		idle = IdleMin
	}
	if idle > DefaultIdle {
		idle = DefaultIdle
	}
	return idle
}

func parsePingSeconds(raw, source string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("ws ping %s=%q invalid, ignored", source, raw)
		return 0, false
	}
	d := time.Duration(n) * time.Second
	switch {
	case d < PingMin:
		log.Printf("ws ping %s=%s below 1s, clamped to 1s", source, d)
		d = PingMin
	case d > PingMax:
		log.Printf("ws ping %s=%s above 60s, clamped to 60s", source, d)
		d = PingMax
	}
	return d, true
}
