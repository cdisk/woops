package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDialWithRetryOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	n := 0
	ws, err := dialWithRetry(ctx, func(context.Context) (*websocket.Conn, error) {
		n++
		if n == 1 {
			return nil, fmt.Errorf("dns timeout")
		}
		return &websocket.Conn{}, nil
	})
	if err != nil || ws == nil {
		t.Fatalf("retry: ws=%v err=%v", ws, err)
	}
	if n != 2 {
		t.Fatalf("attempts %d", n)
	}
}

func TestDialWithRetryStopsWhenLittleTimeLeft(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	n := 0
	start := time.Now()
	_, err := dialWithRetry(ctx, func(context.Context) (*websocket.Conn, error) {
		n++
		time.Sleep(2 * time.Second)
		return nil, fmt.Errorf("dial timeout")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if n != 1 {
		t.Fatalf("attempts %d", n)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("retried too long: %s", time.Since(start))
	}
}

func TestDialWithRetryHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := dialWithRetry(ctx, func(ctx context.Context) (*websocket.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err == nil {
		t.Fatal("expected deadline")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("did not stop at deadline")
	}
}
