package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHandlerAuthAndCIDR(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Username:    "u",
		Password:    "p",
		AllowCIDRs:  []string{"127.0.0.0/8"},
		AllowGlobal: true,
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	ops, _ := parseOpsTarget("10.0.0.1:9200")
	h := &handler{cfg: cfg, ops: ops, log: discardLogger()}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cidr code %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusProxyAuthRequired {
		t.Fatalf("auth code %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("blocked dest code %d body %s", rr.Code, rr.Body.String())
	}
}

func TestTryStartSoftFailBadConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// Missing password: must not panic; soft-fail only.
	TryStart(ctx, Deps{
		Server: "127.0.0.1:9200",
		Raw:    []byte("enabled: true\nusername: only\n"),
	})
}

func TestValidateMissingAuthMessage(t *testing.T) {
	cfg := Config{Enabled: true, Username: "u"}
	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "username and password required") {
		t.Fatalf("msg: %v", err)
	}
}

func TestTryStartSoftFailBind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	raw := []byte(fmt.Sprintf(`
enabled: true
listen: %q
username: u
password: p
allowGlobal: true
`, addr))
	done := make(chan struct{})
	go func() {
		TryStart(ctx, Deps{Server: "10.0.0.1:9200", Raw: raw})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TryStart hung on bind failure")
	}
}

func TestProxyForwardOpsOnly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	upURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	upHost := upURL.Hostname()
	upPort := upURL.Port()
	serverAddr := net.JoinHostPort(upHost, upPort)

	cfg := Config{
		Enabled:     true,
		Listen:      "127.0.0.1:0",
		Username:    "u",
		Password:    "p",
		AllowGlobal: false,
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}

	// Resolve ephemeral listen address first.
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = serve(ctx, cfg, serverAddr, nil, discardLogger())
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", cfg.Listen, 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy did not listen")
		}
		time.Sleep(20 * time.Millisecond)
	}

	proxyURL, err := url.Parse("http://" + cfg.Listen)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   3 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, upstream.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "ok" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}

	badReq, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	badReq.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	badResp, err := client.Do(badReq)
	if err != nil {
		return
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 got %d", badResp.StatusCode)
	}
}

// C → B(proxy+upstream=A) → A(proxy) → target  (CONNECT chain).
func TestProxyUpstreamCONNECTChain(t *testing.T) {
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer targetLn.Close()
	targetAddr := targetLn.Addr().String()
	go func() {
		c, err := targetLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		_, _ = c.Write([]byte("PONG:" + string(buf[:n])))
	}()

	// A: edge proxy, direct dial, ops-only to target.
	aCfg := Config{
		Enabled: true, Listen: "127.0.0.1:0",
		Username: "a", Password: "ap", AllowGlobal: false,
	}
	if err := aCfg.validate(); err != nil {
		t.Fatal(err)
	}
	aln, err := net.Listen("tcp", aCfg.Listen)
	if err != nil {
		t.Fatal(err)
	}
	aCfg.Listen = aln.Addr().String()
	_ = aln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = serve(ctx, aCfg, "http://"+targetAddr, nil, discardLogger())
	}()

	// B: mid proxy, upstream = A.
	bCfg := Config{
		Enabled: true, Listen: "127.0.0.1:0",
		Username: "b", Password: "bp", AllowGlobal: false,
	}
	if err := bCfg.validate(); err != nil {
		t.Fatal(err)
	}
	bln, err := net.Listen("tcp", bCfg.Listen)
	if err != nil {
		t.Fatal(err)
	}
	bCfg.Listen = bln.Addr().String()
	_ = bln.Close()

	aProxyURL, err := url.Parse("http://a:ap@" + aCfg.Listen)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer wg.Done()
		_ = serve(ctx, bCfg, "http://"+targetAddr, aProxyURL, discardLogger())
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

	waitListen := func(addr string) {
		deadline := time.Now().Add(2 * time.Second)
		for {
			c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("listen timeout %s", addr)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitListen(aCfg.Listen)
	waitListen(bCfg.Listen)

	th, tp, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatal(err)
	}
	tport, _ := strconv.Atoi(tp)

	conn, err := dialViaUpstreamCONNECT(
		&url.URL{Scheme: "http", Host: bCfg.Listen, User: url.UserPassword("b", "bp")},
		th, tport,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "PONG:ping" {
		t.Fatalf("got %q", got)
	}
}
