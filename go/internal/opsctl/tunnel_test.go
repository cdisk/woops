package opsctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/protocol/datagram"
)

func TestParseTunnelOptionsDefaultsListenAndSupportsIPv6(t *testing.T) {
	opts, err := ParseTunnelOptions("TCP", ":8080", "[2001:db8::5]:443")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Protocol != "tcp" || opts.ListenHost != "127.0.0.1" || opts.ListenPort != 8080 {
		t.Fatalf("unexpected listen options: %+v", opts)
	}
	if opts.TargetHost != "2001:db8::5" || opts.TargetPort != 443 {
		t.Fatalf("unexpected target options: %+v", opts)
	}
}

func TestParseTunnelOptionsRejectsAmbiguousIPv6AndEmptyTargetHost(t *testing.T) {
	for _, target := range []string{"2001:db8::5:443", ":443"} {
		if _, err := ParseTunnelOptions("udp", "[::1]:1", target); err == nil {
			t.Fatalf("expected target %q to fail", target)
		}
	}
}

func TestNewEphemeralIDIsUUIDv4(t *testing.T) {
	id, err := NewEphemeralID()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^opsctl:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("not UUID v4: %q", id)
	}
}

func TestIsPermanent(t *testing.T) {
	if !IsPermanent(&HTTPError{StatusCode: http.StatusUnauthorized, Body: "no"}) {
		t.Fatal("401 must be permanent")
	}
	if !IsPermanent(errors.New("deploy token revoked")) {
		t.Fatal("revoked token must be permanent")
	}
	if IsPermanent(errors.New("gateway unavailable")) {
		t.Fatal("network outage must be retryable")
	}
}

func TestRunReverseRetriesGatewayFailureAndControlDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var (
		mu         sync.Mutex
		requests   int
		mappingIDs []string
		cancel     context.CancelFunc
		server     *httptest.Server
	)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/opsctl/tickets":
			var request struct {
				Meta map[string]any `json:"meta"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			mu.Lock()
			requests++
			count := requests
			mappingIDs = append(mappingIDs, fmt.Sprint(request.Meta["ephemeralId"]))
			mu.Unlock()
			if count == 1 {
				http.Error(w, "gateway warming up", http.StatusServiceUnavailable)
				return
			}
			if count >= 3 {
				cancel()
				http.Error(w, "stopping test", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"sessionId":"control","ticket":"ticket","browserWs":"`+
				"ws"+strings.TrimPrefix(server.URL, "http")+`/control"}`)
		case "/control":
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			_ = ws.WriteJSON(map[string]any{
				"type": "status", "ok": true, "listenHost": "127.0.0.1", "listenPort": 18080,
			})
			_ = ws.Close()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, stop := context.WithTimeout(context.Background(), 8*time.Second)
	cancel = stop
	defer stop()
	client := NewClient(Config{
		Server: server.URL, Token: "ops_x_y",
		http: server.Client(), dialer: websocket.DefaultDialer,
	})
	err := RunReverse(ctx, client, TunnelOptions{
		Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: 18080,
		TargetHost: "127.0.0.1", TargetPort: 8080, EphemeralID: "opsctl:stable-mapping-id",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests < 3 {
		t.Fatalf("expected retries across both outages, got %d requests", requests)
	}
	for _, id := range mappingIDs {
		if id != "opsctl:stable-mapping-id" {
			t.Fatalf("mapping id changed across reconnects: %#v", mappingIDs)
		}
	}
}

func TestHandleForwardTCPRelaysBinaryWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/opsctl/tickets":
			_, _ = io.WriteString(w, `{"sessionId":"s1","ticket":"t1","browserWs":"`+
				"ws"+strings.TrimPrefix(server.URL, "http")+`/data"}`)
		case "/data":
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer ws.Close()
			for {
				messageType, data, err := ws.ReadMessage()
				if err != nil {
					return
				}
				if err := ws.WriteMessage(messageType, data); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL, Token: "ops_x_y", http: server.Client(), dialer: websocket.DefaultDialer})
	local, peer := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- handleForwardTCP(ctx, client, TunnelOptions{
			Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: 9000,
			TargetHost: "target", TargetPort: 22, EphemeralID: "test-id",
		}, local)
	}()

	_ = peer.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := peer.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 5)
	if _, err := io.ReadFull(peer, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	_ = peer.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TCP relay did not stop")
	}
}

func TestServeForwardUDPRelaysDatagramFrame(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			messageType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(messageType, data); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	destination, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var sink packetSink
	done := make(chan error, 1)
	go func() { done <- serveForwardUDP(ctx, pc, ws, &sink) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		sink.mu.RLock()
		ready := sink.ch != nil
		sink.mu.RUnlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("UDP sink was not installed")
		}
		time.Sleep(time.Millisecond)
	}
	sink.offer(datagram.Encode("127.0.0.1", destination.LocalAddr().(*net.UDPAddr).Port, []byte("ping")))
	_ = destination.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 16)
	n, _, err := destination.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "ping" {
		t.Fatalf("got %q", buf[:n])
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("UDP relay did not stop")
	}
}

func TestServeReverseTCPDialsFixedTarget(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		conn, err := target.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	upgrader := websocket.Upgrader{}
	serverResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverResult <- err
			return
		}
		defer ws.Close()
		if err := ws.WriteMessage(websocket.BinaryMessage, []byte("reverse")); err != nil {
			serverResult <- err
			return
		}
		_, got, err := ws.ReadMessage()
		if err == nil && string(got) != "reverse" {
			err = errors.New("unexpected reverse TCP payload")
		}
		serverResult <- err
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	addr := target.Addr().(*net.TCPAddr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveReverseTCP(ctx, ws, TunnelOptions{TargetHost: addr.IP.String(), TargetPort: addr.Port})
	}()
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reverse TCP relay timed out")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reverse TCP relay did not stop")
	}
}

func TestServeReverseUDPUsesClientAddressInFrames(t *testing.T) {
	target, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		buf := make([]byte, 64)
		n, addr, err := target.ReadFromUDP(buf)
		if err == nil {
			_, _ = target.WriteToUDP(buf[:n], addr)
		}
	}()

	upgrader := websocket.Upgrader{}
	serverResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverResult <- err
			return
		}
		defer ws.Close()
		if err := ws.WriteMessage(websocket.BinaryMessage, datagram.Encode("2001:db8::9", 4567, []byte("udp"))); err != nil {
			serverResult <- err
			return
		}
		_, frame, err := ws.ReadMessage()
		if err == nil {
			host, port, payload, decodeErr := datagram.Decode(frame)
			if decodeErr != nil || host != "2001:db8::9" || port != 4567 || string(payload) != "udp" {
				err = fmt.Errorf("unexpected reverse UDP frame: %s %d %q (%v)", host, port, payload, decodeErr)
			}
		}
		serverResult <- err
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	addr := target.LocalAddr().(*net.UDPAddr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveReverseUDP(ctx, ws, TunnelOptions{TargetHost: addr.IP.String(), TargetPort: addr.Port})
	}()
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reverse UDP relay timed out")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reverse UDP relay did not stop")
	}
}
