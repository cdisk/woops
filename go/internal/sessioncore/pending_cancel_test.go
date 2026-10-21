package sessioncore

import (
	"testing"
	"time"
)

func TestPendingCancelUnblocksWait(t *testing.T) {
	p := NewPendingSessions()
	p.Arm("s1")
	done := make(chan error, 1)
	go func() {
		_, err := p.Wait("s1", 5*time.Second)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if !p.Cancel("s1") {
		t.Fatal("cancel")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancel error")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return")
	}
}
