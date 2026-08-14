package portmap

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ops-bastion/ops/go/internal/protocol/control"
)

func TestReverseTCPListenAcceptAndUnlisten(t *testing.T) {
	m := NewManager(Deps{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	var reported atomic.Bool
	m.Listen(control.PortmapListenPayload{
		MappingID:  "map-1",
		Protocol:   "tcp",
		ListenHost: "127.0.0.1",
		ListenPort: port,
	}, func(st control.PortmapListenStatusPayload) {
		if !st.OK {
			t.Errorf("status err=%s", st.Error)
		}
		reported.Store(true)
	})
	if !reported.Load() {
		t.Fatal("expected status")
	}

	// Port should be bound: second listen same port fails if we use a fresh manager.
	m2 := NewManager(Deps{})
	var fail atomic.Bool
	m2.Listen(control.PortmapListenPayload{
		MappingID:  "map-2",
		Protocol:   "tcp",
		ListenHost: "127.0.0.1",
		ListenPort: port,
	}, func(st control.PortmapListenStatusPayload) {
		if st.OK {
			t.Error("expected bind failure")
		} else {
			fail.Store(true)
		}
	})
	if !fail.Load() {
		t.Fatal("expected conflict status")
	}

	// Idempotent re-listen same mapping succeeds without double-bind.
	var ok2 atomic.Bool
	m.Listen(control.PortmapListenPayload{
		MappingID:  "map-1",
		Protocol:   "tcp",
		ListenHost: "127.0.0.1",
		ListenPort: port,
	}, func(st control.PortmapListenStatusPayload) {
		ok2.Store(st.OK)
	})
	if !ok2.Load() {
		t.Fatal("idempotent listen should ok")
	}

	m.Unlisten("map-1")
	// After unlisten, port should be free again.
	ln2, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("port still busy: %v", err)
	}
	_ = ln2.Close()
}

func TestReverseCloseAllOnControlDrop(t *testing.T) {
	m := NewManager(Deps{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	m.Listen(control.PortmapListenPayload{
		MappingID: "map-x", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: port,
	}, func(control.PortmapListenStatusPayload) {})

	m.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	_, err = d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		t.Fatal("expected closed listener")
	}
}
