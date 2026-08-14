package portmap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/control"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
	"github.com/ops-bastion/ops/go/internal/sessioncore"
)

func TestOpsctlForwardBuildsTunnelWithoutPersistentEntry(t *testing.T) {
	var gotSpec sessioncore.BridgeSpec
	service := NewService(Deps{
		ServeBridge: func(spec sessioncore.BridgeSpec, _ http.ResponseWriter, _ *http.Request) {
			gotSpec = spec
		},
	})
	req := httptest.NewRequest("GET", "/ws/opsctl/portmap-forward/tcp", nil)
	rec := httptest.NewRecorder()
	service.HandleOpsctlForward("tcp")(rec, req)

	if gotSpec.Protocol != "tcp" || gotSpec.OperationType == "" || gotSpec.Prepare == nil {
		t.Fatalf("unexpected bridge spec: %#v", gotSpec)
	}
	raw, _ := json.Marshal(map[string]any{
		"type": "tcp", "assetId": "asset-1", "ephemeral": true,
		"ephemeralId": "opsctl:ephemeral-1", "initiator": "opsctl",
		"direction": "opsctl_to_asset", "protocol": "tcp",
		"listenHost": "127.0.0.1", "listenPort": 15432,
		"targetHost": "10.0.0.8", "targetPort": 5432,
	})
	prepared, err := gotSpec.Prepare(sessioncore.BaseClaims{
		Type: "tcp", AssetID: "asset-1",
	}, raw)
	if err != nil {
		t.Fatal(err)
	}
	params, ok := prepared.Params.(control.TunnelParams)
	if !ok || params.TargetHost != "10.0.0.8" || params.TargetPort != 5432 {
		t.Fatalf("unexpected params: %#v", prepared.Params)
	}
	if service.Manager.has("opsctl:ephemeral-1") {
		t.Fatal("opsctl forward must not enter persistent manager")
	}
	if prepared.StartDetail["mappingId"] != nil || prepared.StartDetail["ephemeral"] != true {
		t.Fatalf("unexpected audit detail: %#v", prepared.StartDetail)
	}
}

func TestEphemeralUDPRelayPreservesLargeDatagramAndCountsPayload(t *testing.T) {
	upgrader := websocket.Upgrader{}
	agentServerCh := make(chan *websocket.Conn, 1)
	ctlServerCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		if r.URL.Path == "/agent" {
			agentServerCh <- ws
		} else {
			ctlServerCh <- ws
		}
	}))
	defer server.Close()
	dial := func(path string) *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(server.URL, "http")+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	agentClient, ctlClient := dial("/agent"), dial("/ctl")
	defer agentClient.Close()
	defer ctlClient.Close()
	agentServer, ctlServer := <-agentServerCh, <-ctlServerCh

	var bytesIn, bytesOut int64
	done := make(chan struct{})
	go func() {
		relayDatagramPeers(agentServer, ctlServer, &bytesIn, &bytesOut)
		close(done)
	}()
	payload := bytes.Repeat([]byte{0x5a}, 60*1024)
	frame := datagram.Encode("10.0.0.9", 4567, payload)
	if gotSize := datagramPayloadSize(websocket.BinaryMessage, frame); gotSize != int64(len(payload)) {
		t.Fatalf("forward UDP sizer counted %d bytes, want %d", gotSize, len(payload))
	}
	if err := agentClient.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	_, got, err := ctlClient.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatalf("large datagram changed: got %d bytes, want %d", len(got), len(frame))
	}
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt64(&bytesIn) != int64(len(payload)) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if atomic.LoadInt64(&bytesIn) != int64(len(payload)) {
		t.Fatalf("audited bytes include framing: got %d want %d", bytesIn, len(payload))
	}
	_ = agentClient.Close()
	_ = ctlClient.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("datagram relay did not stop")
	}
}

func TestEphemeralListenStatusIsIsolatedFromPersistentManager(t *testing.T) {
	service := NewService(Deps{})
	entry := &ephemeralEntry{
		claims: opsctlClaims{
			BaseClaims:  sessioncore.BaseClaims{AssetID: "asset-1", Type: typeOpsctlReverseControl},
			Ephemeral:   true,
			EphemeralID: "opsctl:ephemeral-1",
			Protocol:    "tcp",
		},
		statusCh: make(chan control.PortmapListenStatusPayload, 1),
	}
	service.ephemeral[ephemeralKey("asset-1", "opsctl:ephemeral-1")] = entry

	handled := service.handleEphemeralListenStatus("asset-1", control.PortmapListenStatusPayload{
		MappingID: "opsctl:ephemeral-1", OK: true, ListenPort: 5432,
	})
	if !handled {
		t.Fatal("ephemeral status was not handled")
	}
	if service.Manager.has("opsctl:ephemeral-1") || len(service.Manager.list()) != 0 {
		t.Fatal("ephemeral mapping leaked into persistent manager")
	}
	select {
	case status := <-entry.statusCh:
		if !status.OK || status.ListenPort != 5432 {
			t.Fatalf("unexpected status: %#v", status)
		}
	default:
		t.Fatal("status was not delivered")
	}
}

func TestEphemeralIDsAreScopedByAsset(t *testing.T) {
	service := NewService(Deps{})
	newEntry := func(assetID string) *ephemeralEntry {
		return &ephemeralEntry{
			claims: opsctlClaims{
				BaseClaims:  sessioncore.BaseClaims{AssetID: assetID},
				Ephemeral:   true,
				EphemeralID: "opsctl:same-id",
				Protocol:    "tcp",
			},
			statusCh: make(chan control.PortmapListenStatusPayload, 1),
		}
	}
	first, second := newEntry("asset-1"), newEntry("asset-2")
	service.ephemeral[ephemeralKey("asset-1", "opsctl:same-id")] = first
	service.ephemeral[ephemeralKey("asset-2", "opsctl:same-id")] = second

	if !service.handleEphemeralListenStatus("asset-2", control.PortmapListenStatusPayload{
		MappingID: "opsctl:same-id", OK: true,
	}) {
		t.Fatal("asset-scoped status not handled")
	}
	select {
	case <-second.statusCh:
	default:
		t.Fatal("status was not delivered to matching asset")
	}
	select {
	case <-first.statusCh:
		t.Fatal("status crossed assets")
	default:
	}
}
