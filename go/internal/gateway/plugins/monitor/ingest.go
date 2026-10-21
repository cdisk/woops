package monitor

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
)

const ingestQueueSize = 1024

type ingestJob struct {
	assetID string
	body    map[string]any
}

// Ingestor posts metric samples to control-api off the WebSocket read loop.
// A full queue drops the sample instead of blocking the caller.
type Ingestor struct {
	post        Poster
	ch          chan ingestJob
	dropped     int64
	mu          sync.Mutex
	lastDropLog time.Time
}

func NewIngestor(post Poster) *Ingestor {
	return newIngestor(post, 4)
}

func newIngestor(post Poster, workers int) *Ingestor {
	g := &Ingestor{post: post, ch: make(chan ingestJob, ingestQueueSize)}
	for i := 0; i < workers; i++ {
		go g.worker()
	}
	return g
}

// Enqueue returns false when the queue is full. It never blocks.
func (g *Ingestor) Enqueue(assetID, token string, payload monitorproto.PointsPayload) bool {
	if g == nil {
		return false
	}
	job := ingestJob{assetID: assetID, body: ingestBody(assetID, token, payload)}
	select {
	case g.ch <- job:
		return true
	default:
		g.noteDrop(assetID)
		return false
	}
}

func (g *Ingestor) worker() {
	for job := range g.ch {
		if g.post == nil {
			continue
		}
		if err := g.post.PostJSON("/api/internal/metrics/ingest", job.body, nil); err != nil {
			log.Printf("metrics ingest failed asset=%s: %v", job.assetID, err)
		}
	}
}

func (g *Ingestor) noteDrop(assetID string) {
	n := atomic.AddInt64(&g.dropped, 1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.lastDropLog.IsZero() && time.Since(g.lastDropLog) < 30*time.Second {
		return
	}
	g.lastDropLog = time.Now()
	log.Printf("metrics ingest queue full asset=%s dropped=%d", assetID, n)
}

func ingestBody(assetID, token string, payload monitorproto.PointsPayload) map[string]any {
	points := make([]map[string]any, 0, len(payload.Points))
	for _, p := range payload.Points {
		points = append(points, map[string]any{
			"itemId":   p.ItemID,
			"instance": p.Instance,
			"value":    p.Value,
		})
	}
	return map[string]any{
		"assetId":          assetID,
		"agentToken":       token,
		"collectedAt":      payload.CollectedAt,
		"points":           points,
		"completeFamilies": payload.CompleteFamilies,
	}
}
