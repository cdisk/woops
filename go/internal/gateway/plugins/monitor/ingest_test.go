package monitor

import (
	"sync/atomic"
	"testing"
	"time"

	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
)

type posterFunc func(path string, body any, out any) error

func (f posterFunc) PostJSON(path string, body any, out any) error { return f(path, body, out) }

func TestIngestorDropsWhenFull(t *testing.T) {
	g := newIngestor(posterFunc(func(string, any, any) error { return nil }), 0)
	payload := monitorproto.PointsPayload{CollectedAt: "t"}
	for i := 0; i < ingestQueueSize; i++ {
		if !g.Enqueue("asset", "token", payload) {
			t.Fatalf("dropped early at %d", i)
		}
	}
	start := time.Now()
	if g.Enqueue("asset", "token", payload) {
		t.Fatal("expected drop")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("enqueue blocked for %s", elapsed)
	}
	if atomic.LoadInt64(&g.dropped) != 1 {
		t.Fatalf("dropped %d", g.dropped)
	}
}

func TestIngestorPostsBody(t *testing.T) {
	done := make(chan map[string]any, 1)
	g := newIngestor(posterFunc(func(path string, body any, out any) error {
		if path != "/api/internal/metrics/ingest" {
			t.Errorf("path %s", path)
		}
		done <- body.(map[string]any)
		return nil
	}), 1)
	ok := g.Enqueue("asset-1", "tok-1", monitorproto.PointsPayload{
		CollectedAt:      "2026-10-21T00:00:00Z",
		Points:           []monitorproto.Point{{ItemID: "cpu.usage_percent", Instance: "", Value: 1.5}},
		CompleteFamilies: []string{"cpu"},
	})
	if !ok {
		t.Fatal("enqueue")
	}
	select {
	case body := <-done:
		if body["assetId"] != "asset-1" || body["agentToken"] != "tok-1" {
			t.Fatalf("ids %#v", body)
		}
		if body["collectedAt"] != "2026-10-21T00:00:00Z" {
			t.Fatalf("collectedAt %#v", body["collectedAt"])
		}
		fams, _ := body["completeFamilies"].([]string)
		if len(fams) != 1 || fams[0] != "cpu" {
			t.Fatalf("families %#v", body["completeFamilies"])
		}
		points, _ := body["points"].([]map[string]any)
		if len(points) != 1 || points[0]["itemId"] != "cpu.usage_percent" {
			t.Fatalf("points %#v", body["points"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not post")
	}
}
