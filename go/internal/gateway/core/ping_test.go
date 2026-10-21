package core

import (
	"testing"
	"time"

	"github.com/ops-bastion/ops/go/internal/wsutil"
)

func TestPingForMapsProtocols(t *testing.T) {
	s := New(Config{Ping: wsutil.PingConfig{
		Global: 25 * time.Second,
		ByKind: map[string]time.Duration{
			"shell":   3 * time.Second,
			"file":    4 * time.Second,
			"desktop": 6 * time.Second,
			"portmap": 7 * time.Second,
		},
	}})
	cases := map[string]time.Duration{
		"control":      25 * time.Second,
		"shell":        3 * time.Second,
		"exec":         25 * time.Second,
		"filemanager":  4 * time.Second,
		"filetransfer": 4 * time.Second,
		"rdp":          6 * time.Second,
		"vnc":          6 * time.Second,
		"tcp":          7 * time.Second,
		"udp":          7 * time.Second,
	}
	for kind, want := range cases {
		if got := s.PingFor(kind); got != want {
			t.Fatalf("%s=%s want %s", kind, got, want)
		}
	}
}
