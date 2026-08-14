package exec

import "testing"

func TestExecSnifferRunAndDone(t *testing.T) {
	var events []string
	var details []map[string]any
	s := NewSniffer(func(eventType string, detail map[string]any) {
		events = append(events, eventType)
		details = append(details, detail)
	})
	s.ObserveClient([]byte(`{"type":"run","command":"echo hi","cwd":"/tmp","timeoutSec":30}`))
	if len(events) != 1 || events[0] != "RUN" {
		t.Fatalf("events=%v", events)
	}
	if details[0]["command"] != "echo hi" || details[0]["cwd"] != "/tmp" {
		t.Fatalf("detail=%v", details[0])
	}
	// Second run ignored (one-shot session).
	s.ObserveClient([]byte(`{"type":"run","command":"echo again"}`))
	if len(events) != 1 {
		t.Fatalf("expected one RUN, got %d", len(events))
	}
	code := 0
	s.ObserveServer([]byte(`{"type":"done","exitCode":0,"durationMs":12}`))
	end := s.EndDetail()
	if end["exitCode"] != 0 && end["exitCode"] != code {
		// exitCode stored as int via pointer deref - JSON unmarshal gives *int
	}
	if end["command"] != "echo hi" {
		t.Fatalf("end=%v", end)
	}
	if v, ok := end["exitCode"].(int); !ok || v != 0 {
		t.Fatalf("exitCode=%v (%T)", end["exitCode"], end["exitCode"])
	}
	if !s.Success() {
		t.Fatal("expected success")
	}
}

func TestExecSnifferError(t *testing.T) {
	s := NewSniffer(func(string, map[string]any) {})
	s.ObserveClient([]byte(`{"type":"run","command":"false"}`))
	s.ObserveServer([]byte(`{"type":"error","error":"boom"}`))
	if s.Success() {
		t.Fatal("expected failure")
	}
	end := s.EndDetail()
	if end["error"] != "boom" {
		t.Fatalf("end=%v", end)
	}
}
