package monitor

import (
	"sync/atomic"
	"testing"
	"time"

	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
)

func TestReporterOneLoop(t *testing.T) {
	var n atomic.Int32
	r := NewReporter(20 * time.Millisecond)
	r.collect = func() Sample {
		return Sample{Points: []monitorproto.Point{{ItemID: "cpu.usage_percent", Value: 1}}}
	}
	send := func(typ, requestID string, payload any) error {
		if typ != "metrics_points" {
			t.Errorf("type %s", typ)
			return nil
		}
		body, ok := payload.(monitorproto.PointsPayload)
		if !ok || len(body.Points) != 1 {
			t.Errorf("payload %#v", payload)
		}
		n.Add(1)
		return nil
	}
	r.OnConnect(send)
	waitFor(&n, 1)
	r.OnDisconnect()
	time.Sleep(15 * time.Millisecond)
	stopped := n.Load()
	time.Sleep(80 * time.Millisecond)
	if n.Load() > stopped+1 {
		t.Fatalf("loop still running: %d -> %d", stopped, n.Load())
	}

	r.OnConnect(send)
	r.OnConnect(send)
	time.Sleep(70 * time.Millisecond)
	got := n.Load() - stopped
	if got < 1 {
		t.Fatal("reconnect did not report")
	}
	if got > 8 {
		t.Fatalf("two loops after reconnect: +%d", got)
	}
	r.OnDisconnect()
}

func waitFor(n *atomic.Int32, min int32) {
	deadline := time.Now().Add(time.Second)
	for n.Load() < min && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}
