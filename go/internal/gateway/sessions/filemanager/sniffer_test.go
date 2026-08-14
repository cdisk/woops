package filemanager

import (
	"testing"
)

type sniffedEvent struct {
	eventType string
	detail    map[string]any
}

func newTestSniffer() (*Sniffer, *[]sniffedEvent) {
	var got []sniffedEvent
	s := NewSniffer(func(eventType string, detail map[string]any) {
		got = append(got, sniffedEvent{eventType, detail})
	})
	return s, &got
}

func TestFileSnifferEmitsPathEvents(t *testing.T) {
	s, got := newTestSniffer()
	s.Observe([]byte(`{"id":1,"method":"list","params":{"path":"/tmp"}}`))
	s.Observe([]byte(`{"id":2,"method":"rename","params":{"path":"/tmp/a","to":"/tmp/b"}}`))
	s.Observe([]byte(`{"id":3,"method":"roots","params":{}}`))
	s.Flush()

	if len(*got) != 3 {
		t.Fatalf("want 3 events, got %d (%v)", len(*got), *got)
	}
	if (*got)[0].eventType != "LIST" || (*got)[0].detail["path"] != "/tmp" {
		t.Fatalf("event 0 = %v", (*got)[0])
	}
	if (*got)[1].eventType != "RENAME" || (*got)[1].detail["to"] != "/tmp/b" {
		t.Fatalf("event 1 = %v", (*got)[1])
	}
	if (*got)[2].eventType != "ROOTS" {
		t.Fatalf("event 2 = %v", (*got)[2])
	}
}

func TestFileSnifferIgnoresContentRPC(t *testing.T) {
	s, got := newTestSniffer()
	s.Observe([]byte(`{"id":1,"method":"write","params":{"path":"/tmp/x","offset":0,"data":"YQ=="}}`))
	s.Observe([]byte(`{"id":2,"method":"read","params":{"path":"/tmp/x","offset":0,"length":10}}`))
	s.Flush()
	if len(*got) != 0 {
		t.Fatalf("content RPC should be ignored: %v", *got)
	}
}

func TestFileSnifferIgnoresGarbage(t *testing.T) {
	s, got := newTestSniffer()
	s.Observe([]byte("not json"))
	s.Observe([]byte(`{"id":1}`))
	s.Observe(nil)
	var nilSniffer *Sniffer
	nilSniffer.Observe([]byte(`{"method":"list","params":{"path":"/"}}`))
	nilSniffer.Flush()
	if len(*got) != 0 {
		t.Fatalf("garbage produced events: %v", *got)
	}
}
