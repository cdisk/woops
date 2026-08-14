package filetransfer

import (
	"testing"

	"github.com/gorilla/websocket"
	filetransferproto "github.com/ops-bastion/ops/go/internal/protocol/filetransfer"
)

type sniffedEvent struct {
	eventType string
	detail    map[string]any
}

func TestTransferSnifferAggregatesBinary(t *testing.T) {
	var events []sniffedEvent
	s := NewSniffer(func(eventType string, detail map[string]any) {
		events = append(events, sniffedEvent{eventType, detail})
	}, Opts{
		TransferID: "t1",
		Direction:  "upload",
		Path:       "/tmp/a",
	})
	frame, err := filetransferproto.EncodeChunk(0, make([]byte, 1000))
	if err != nil {
		t.Fatal(err)
	}
	s.Observe(websocket.BinaryMessage, frame, true)
	frame2, _ := filetransferproto.EncodeChunk(1000, make([]byte, 500))
	s.Observe(websocket.BinaryMessage, frame2, true)
	if len(events) != 0 {
		t.Fatalf("premature events: %v", events)
	}
	s.Flush()
	if len(events) != 1 || events[0].eventType != "WRITE" {
		t.Fatalf("events=%v", events)
	}
	if events[0].detail["bytes"] != int64(1500) || events[0].detail["chunks"] != 2 {
		t.Fatalf("detail=%v", events[0].detail)
	}
	if _, ok := events[0].detail["data"]; ok {
		t.Fatal("payload leaked")
	}
}

func TestTransferSnifferControlEvents(t *testing.T) {
	var events []sniffedEvent
	s := NewSniffer(func(eventType string, detail map[string]any) {
		events = append(events, sniffedEvent{eventType, detail})
	}, Opts{TransferID: "t2", Direction: "download", Path: "/x"})
	raw, _ := filetransferproto.EncodeControl(filetransferproto.Control{
		Type: filetransferproto.CtrlStatus, Offset: 10, Size: 99, Resumed: true,
	})
	s.Observe(websocket.TextMessage, raw, false)
	if len(events) != 1 || events[0].eventType != "TRANSFER_STATUS" {
		t.Fatalf("events=%v", events)
	}
	if events[0].detail["resumed"] != true {
		t.Fatalf("detail=%v", events[0].detail)
	}
}
