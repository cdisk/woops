package sessioncore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPendingSessionsDeliverAfterArm(t *testing.T) {
	p := NewPendingSessions()
	p.Arm("s1")

	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		if !p.Deliver("s1", ws) {
			t.Error("Deliver returned false")
			ws.Close()
		}
	}))
	defer srv.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):], nil)
		if err != nil {
			t.Errorf("dial: %v", err)
			return
		}
		_ = ws // delivered to Wait
	}()

	got, err := p.Wait("s1", 2*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	defer got.Close()
	<-done
}

func TestPendingSessionsWaitTimeout(t *testing.T) {
	p := NewPendingSessions()
	_, err := p.Wait("missing", 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout")
	}
}
