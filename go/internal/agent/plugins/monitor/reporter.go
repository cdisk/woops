package monitor

import (
	"context"
	"sync"
	"time"

	monitorproto "github.com/ops-bastion/ops/go/internal/protocol/monitor"
)

// Sender writes one control-channel message. It shares the control write lock.
type Sender func(typ, requestID string, payload any) error

// Reporter publishes metrics on the Agent control connection.
// OnConnect replaces any previous loop so a reconnect does not double-report.
type Reporter struct {
	interval time.Duration
	collect  func() Sample
	mu       sync.Mutex
	cancel   context.CancelFunc
}

func NewReporter(interval time.Duration) *Reporter {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Reporter{interval: interval, collect: Collect}
}

func (r *Reporter) OnConnect(send Sender) {
	if r == nil || send == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	interval := r.interval
	collect := r.collect
	r.mu.Unlock()
	go runReportLoop(ctx, interval, send, collect)
}

func (r *Reporter) OnDisconnect() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func runReportLoop(ctx context.Context, interval time.Duration, send Sender, collect func() Sample) {
	if collect == nil {
		collect = Collect
	}
	sendOnce := func() {
		if ctx.Err() != nil {
			return
		}
		sample := collect()
		_ = send("metrics_points", "", monitorproto.PointsPayload{
			CollectedAt:      time.Now().UTC().Format(time.RFC3339),
			Points:           sample.Points,
			CompleteFamilies: sample.CompleteFamilies,
		})
	}
	sendOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendOnce()
		}
	}
}
