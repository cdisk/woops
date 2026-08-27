package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
)

func TestBridgeMutualHMAC(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	listener, dialer := net.Pipe()
	defer listener.Close()
	defer dialer.Close()
	errCh := make(chan error, 1)
	go func() { errCh <- listenerHandshake(listener, key) }()
	if err := dialerHandshake(dialer, key); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestBridgeMutualHMACRejectsWrongKey(t *testing.T) {
	listener, dialer := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- listenerHandshake(listener, []byte(strings.Repeat("a", 32)))
		_ = listener.Close()
	}()
	if err := dialerHandshake(dialer, []byte(strings.Repeat("b", 32))); err == nil {
		t.Fatal("wrong key should fail")
	}
	_ = dialer.Close()
	if err := <-errCh; err == nil {
		t.Fatal("listener should reject wrong key")
	}
}

func TestConcurrentCONNECTStreamsThroughCarrier(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go func() {
		for {
			conn, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(conn)
		}
	}()

	serverConn, clientConn := net.Pipe()
	serverSession, err := yamux.Server(serverConn, yamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := yamux.Client(clientConn, yamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	defer clientSession.Close()

	parents := newCarrierPool()
	parents.add(serverSession)
	defer parents.close()
	ops, err := parseOpsTarget("http://" + echoLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	targetHandler := &handler{
		cfg:         Config{AllowGlobal: false},
		ops:         ops,
		bridgeEntry: true,
		log:         discardLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	targetDone := make(chan struct{})
	go func() {
		serveTargetSession(ctx, clientSession, targetHandler)
		close(targetDone)
	}()

	proxyCfg := Config{
		Enabled:     true,
		Username:    "u",
		Password:    "p",
		AllowGlobal: false,
	}
	proxyHandler := &handler{
		cfg:          proxyCfg,
		ops:          ops,
		upstream:     &url.URL{Scheme: "http", Host: "127.0.0.1:3128"},
		parent:       parents,
		upstreamSelf: true,
		log:          discardLogger(),
	}
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyServer := &http.Server{Handler: proxyHandler}
	go proxyServer.Serve(proxyLn)
	defer func() {
		_ = proxyServer.Close()
		cancel()
		<-targetDone
	}()

	targetHost, targetPortText, _ := net.SplitHostPort(echoLn.Addr().String())
	var targetPort int
	_, _ = fmt.Sscanf(targetPortText, "%d", &targetPort)
	proxyURL := &url.URL{
		Scheme: "http",
		Host:   proxyLn.Addr().String(),
		User:   url.UserPassword("u", "p"),
	}

	const clients = 12
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := dialViaUpstreamCONNECT(proxyURL, targetHost, targetPort)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			want := fmt.Sprintf("stream-%d", i)
			if _, err := conn.Write([]byte(want)); err != nil {
				errs <- err
				return
			}
			got := make([]byte, len(want))
			if _, err := io.ReadFull(conn, got); err != nil {
				errs <- err
				return
			}
			if string(got) != want {
				errs <- fmt.Errorf("got %q want %q", got, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestBridgeEntryUsesOwnAllowGlobalPolicy(t *testing.T) {
	ops, _ := parseOpsTarget("10.0.0.1:9200")
	h := &handler{
		cfg:         Config{AllowGlobal: false},
		ops:         ops,
		bridgeEntry: true,
		log:         discardLogger(),
	}
	req, _ := http.NewRequest(http.MethodGet, "http://8.8.8.8/", nil)
	req.RemoteAddr = "bridge:0"
	rr := newResponseRecorder()
	h.ServeHTTP(rr, req)
	if rr.status != http.StatusForbidden {
		t.Fatalf("status %d", rr.status)
	}
}

func TestTargetOnlyReconnectsAndServesHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "through-bridge")
	}))
	defer upstream.Close()

	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bridgeAddr := reserved.Addr().String()
	_ = reserved.Close()
	keyText := strings.Repeat("12", 32)
	listenerCfg := BridgeConfig{Enabled: true, Listen: bridgeAddr, Key: keyText}
	if err := listenerCfg.validate(); err != nil {
		t.Fatal(err)
	}
	targetCfg := BridgeConfig{
		Enabled: true,
		Targets: []BridgeTarget{{Address: bridgeAddr, Key: keyText}},
	}
	if err := targetCfg.validate(); err != nil {
		t.Fatal(err)
	}
	ops, err := parseOpsTarget(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	targetHandler := &handler{
		cfg:         Config{AllowGlobal: false},
		ops:         ops,
		bridgeEntry: true,
		log:         discardLogger(),
	}
	parents := newCarrierPool()
	ctx, cancel := context.WithCancel(context.Background())
	listenerDone := make(chan error, 1)
	targetDone := make(chan struct{})
	go func() {
		listenerDone <- serveBridgeListener(ctx, listenerCfg, parents, discardLogger())
	}()
	go func() {
		runBridgeTarget(ctx, targetCfg.Targets[0], targetHandler, discardLogger())
		close(targetDone)
	}()
	defer func() {
		cancel()
		select {
		case err := <-listenerDone:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("listener did not stop")
		}
		select {
		case <-targetDone:
		case <-time.After(3 * time.Second):
			t.Error("target did not stop")
		}
	}()

	waitCarrier := func(want bool, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for parents.available() != want {
			if time.Now().After(deadline) {
				t.Fatalf("carrier available=%v, want %v", parents.available(), want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitCarrier(true, 4*time.Second)

	request := func() {
		t.Helper()
		stream, err := parents.open()
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		_ = stream.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := fmt.Fprintf(stream, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", upstream.URL, strings.TrimPrefix(upstream.URL, "http://")); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(stream), &http.Request{Method: http.MethodGet})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "through-bridge" {
			t.Fatalf("body %q", body)
		}
	}
	request()

	parents.close()
	waitCarrier(false, time.Second)
	waitCarrier(true, 4*time.Second)
	request()
}

func TestSelfGatewayProxyWithoutParentReturnsFast502(t *testing.T) {
	ops, _ := parseOpsTarget("https://gw.example:9200")
	upstream, _ := url.Parse("http://127.0.0.1:3128")
	h := &handler{
		cfg:          Config{Username: "u", Password: "p", AllowGlobal: false},
		ops:          ops,
		upstream:     upstream,
		upstreamSelf: true,
		log:          discardLogger(),
	}
	req, _ := http.NewRequest(http.MethodGet, "https://gw.example:9200/", nil)
	req.RemoteAddr = "127.0.0.1:1000"
	req.Header.Set("Proxy-Authorization", "Basic dTpw")
	rr := newResponseRecorder()
	start := time.Now()
	h.ServeHTTP(rr, req)
	if rr.status != http.StatusBadGateway {
		t.Fatalf("status %d", rr.status)
	}
	if time.Since(start) > time.Second {
		t.Fatal("self upstream should fail fast")
	}
}

func TestCarrierCascadeContinuesThroughParent(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoLn.Close()
	go func() {
		conn, err := echoLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()
	ops, _ := parseOpsTarget("http://" + echoLn.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	topServerConn, topClientConn := net.Pipe()
	topServer, _ := yamux.Server(topServerConn, yamuxConfig())
	topClient, _ := yamux.Client(topClientConn, yamuxConfig())
	defer topServer.Close()
	defer topClient.Close()
	topParents := newCarrierPool()
	topParents.add(topServer)
	defer topParents.close()
	go serveTargetSession(ctx, topClient, &handler{
		cfg: Config{AllowGlobal: false}, ops: ops, bridgeEntry: true, log: discardLogger(),
	})

	middleServerConn, middleClientConn := net.Pipe()
	middleServer, _ := yamux.Server(middleServerConn, yamuxConfig())
	middleClient, _ := yamux.Client(middleClientConn, yamuxConfig())
	defer middleServer.Close()
	defer middleClient.Close()
	childParents := newCarrierPool()
	childParents.add(middleServer)
	defer childParents.close()
	go serveTargetSession(ctx, middleClient, &handler{
		cfg: Config{AllowGlobal: false}, ops: ops, parent: topParents, bridgeEntry: true, log: discardLogger(),
	})

	stream, err := childParents.open()
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintf(stream, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", echoLn.Addr(), echoLn.Addr()); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(stream)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, err := stream.Write([]byte("cascade")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len("cascade"))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "cascade" {
		t.Fatalf("got %q", got)
	}
}

type responseRecorder struct {
	header http.Header
	status int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header)}
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
}
func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return len(p), nil
}
