package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ops-bastion/ops/go/internal/tlsutil"
)

const dialTimeout = 15 * time.Second

type handler struct {
	cfg          Config
	ops          opsTarget
	upstream     *url.URL // optional: chain CONNECT/forward via gatewayProxy
	parent       *carrierPool
	bridgeEntry  bool
	upstreamSelf bool
	log          *log.Logger
}

func serve(ctx context.Context, cfg Config, serverAddr string, upstream *url.URL, plog *log.Logger) error {
	return serveWithCarrier(ctx, cfg, serverAddr, upstream, nil, plog)
}

func serveWithCarrier(ctx context.Context, cfg Config, serverAddr string, upstream *url.URL, parent *carrierPool, plog *log.Logger) error {
	if plog == nil {
		plog = discardLogger()
	}
	ops, err := parseOpsTarget(serverAddr)
	if err != nil {
		return fmt.Errorf("ops target: %w", err)
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Listen, err)
	}
	if upstream != nil {
		plog.Printf("proxy listening listen=%s allowGlobal=%v upstream=%s",
			cfg.Listen, cfg.AllowGlobal, tlsutil.RedactProxyURL(upstream))
	} else {
		plog.Printf("proxy listening listen=%s allowGlobal=%v", cfg.Listen, cfg.AllowGlobal)
	}

	h := &handler{
		cfg:          cfg,
		ops:          ops,
		upstream:     upstream,
		parent:       parent,
		upstreamSelf: sameListenAddress(upstream, cfg.Listen),
		log:          plog,
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-errCh
		plog.Printf("proxy stopped")
		return nil
	case err := <-errCh:
		if err == nil || err == http.ErrServerClosed {
			plog.Printf("proxy stopped")
			return nil
		}
		return err
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	client := clientIPString(r.RemoteAddr)
	start := time.Now()

	if !h.bridgeEntry && !h.cfg.clientAllowed(remoteIP(r.RemoteAddr)) {
		h.log.Printf("proxy FAIL method=%s client=%s target=- status=403 reason=cidr_denied durationMs=%d",
			r.Method, client, elapsedMs(start))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !h.bridgeEntry && !h.checkAuth(r) {
		h.log.Printf("proxy FAIL method=%s client=%s target=- status=407 reason=auth_required durationMs=%d",
			r.Method, client, elapsedMs(start))
		w.Header().Set("Proxy-Authenticate", `Basic realm="woops-agent-proxy"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r, client, start)
		return
	}
	h.handleForward(w, r, client, start)
}

func (h *handler) checkAuth(r *http.Request) bool {
	const prefix = "Basic "
	auth := r.Header.Get("Proxy-Authorization")
	if auth == "" || !strings.HasPrefix(auth, prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(auth[len(prefix):]))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	return user == h.cfg.Username && pass == h.cfg.Password
}

func (h *handler) handleConnect(w http.ResponseWriter, r *http.Request, client string, start time.Time) {
	host, port, err := splitHostPort(r.Host, 443)
	if err != nil {
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%q status=400 reason=bad_request durationMs=%d",
			client, r.Host, elapsedMs(start))
		http.Error(w, "bad host", http.StatusBadRequest)
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	h.log.Printf("proxy START method=CONNECT client=%s target=%s", client, target)

	if err := h.cfg.authorizeDest(host, port, h.ops); err != nil {
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=403 reason=dest_not_allowed durationMs=%d",
			client, target, elapsedMs(start))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if h.parent != nil && h.parent.available() {
		h.handleConnectCarrier(w, r, client, target, start)
		return
	}
	if h.upstreamSelf && !h.bridgeEntry {
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=502 reason=parent_unavailable durationMs=%d",
			client, target, elapsedMs(start))
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	if sameProxyTarget(h.upstream, host, port) {
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=403 reason=upstream_loop durationMs=%d",
			client, target, elapsedMs(start))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var up net.Conn
	if h.upstream != nil && !h.upstreamSelf {
		up, err = dialViaUpstreamCONNECT(h.upstream, host, port)
		if err != nil {
			h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=502 reason=dial_failed durationMs=%d",
				client, target, elapsedMs(start))
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
	} else {
		dest, err := h.cfg.resolveAndCheckDest(host, port, h.ops)
		if err != nil {
			h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=403 reason=dest_not_allowed durationMs=%d",
				client, target, elapsedMs(start))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		up, err = net.DialTimeout("tcp", dest, dialTimeout)
		if err != nil {
			h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=502 reason=dial_failed durationMs=%d",
				client, target, elapsedMs(start))
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = up.Close()
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=500 reason=hijack_unsupported durationMs=%d",
			client, target, elapsedMs(start))
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, buf, err := hj.Hijack()
	if err != nil {
		_ = up.Close()
		h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=500 reason=hijack_failed durationMs=%d",
			client, target, elapsedMs(start))
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if buf.Reader.Buffered() > 0 {
		if _, err := io.Copy(up, buf); err != nil {
			_ = up.Close()
			_ = clientConn.Close()
			h.log.Printf("proxy FAIL method=CONNECT client=%s target=%s status=502 reason=dial_failed durationMs=%d",
				client, target, elapsedMs(start))
			return
		}
	}
	in, out := relayCounted(clientConn, up)
	h.log.Printf("proxy END method=CONNECT client=%s target=%s durationMs=%d bytesIn=%d bytesOut=%d",
		client, target, elapsedMs(start), in, out)
}

func (h *handler) handleForward(w http.ResponseWriter, r *http.Request, client string, start time.Time) {
	if !r.URL.IsAbs() {
		h.log.Printf("proxy FAIL method=%s client=%s target=- status=400 reason=bad_request durationMs=%d",
			r.Method, client, elapsedMs(start))
		http.Error(w, "absolute URI required", http.StatusBadRequest)
		return
	}
	host := r.URL.Hostname()
	port := 80
	if p := r.URL.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			h.log.Printf("proxy FAIL method=%s client=%s target=- status=400 reason=bad_request durationMs=%d",
				r.Method, client, elapsedMs(start))
			http.Error(w, "bad host", http.StatusBadRequest)
			return
		}
		port = n
	} else if strings.EqualFold(r.URL.Scheme, "https") {
		port = 443
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	h.log.Printf("proxy START method=%s client=%s target=%s", r.Method, client, target)

	if err := h.cfg.authorizeDest(host, port, h.ops); err != nil {
		h.log.Printf("proxy FAIL method=%s client=%s target=%s status=403 reason=dest_not_allowed durationMs=%d",
			r.Method, client, target, elapsedMs(start))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if h.parent != nil && h.parent.available() {
		h.handleForwardCarrier(w, r, client, target, start)
		return
	}
	if h.upstreamSelf && !h.bridgeEntry {
		h.log.Printf("proxy FAIL method=%s client=%s target=%s status=502 reason=parent_unavailable durationMs=%d",
			r.Method, client, target, elapsedMs(start))
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	if sameProxyTarget(h.upstream, host, port) {
		h.log.Printf("proxy FAIL method=%s client=%s target=%s status=403 reason=upstream_loop durationMs=%d",
			r.Method, client, target, elapsedMs(start))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.URL.Scheme = r.URL.Scheme
	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	outReq.Header.Del("Proxy-Authorization")
	outReq.Header.Del("Proxy-Connection")
	// Never log or forward credentials in query; strip query from local logging only
	// (request still goes as-is for functional proxying - body/headers not logged).

	tr := &http.Transport{
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: dialTimeout,
	}

	if h.upstream != nil && !h.upstreamSelf {
		outReq.URL.Host = r.URL.Host
		if outReq.URL.Host == "" {
			outReq.URL.Host = net.JoinHostPort(host, strconv.Itoa(port))
		}
		tr.Proxy = http.ProxyURL(h.upstream)
	} else {
		dest, err := h.cfg.resolveAndCheckDest(host, port, h.ops)
		if err != nil {
			h.log.Printf("proxy FAIL method=%s client=%s target=%s status=403 reason=dest_not_allowed durationMs=%d",
				r.Method, client, target, elapsedMs(start))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		outReq.URL.Host = dest
		tr.Proxy = nil
		tr.DialContext = (&net.Dialer{Timeout: dialTimeout}).DialContext
	}

	resp, err := tr.RoundTrip(outReq)
	if err != nil {
		h.log.Printf("proxy FAIL method=%s client=%s target=%s status=502 reason=dial_failed durationMs=%d",
			r.Method, client, target, elapsedMs(start))
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	h.log.Printf("proxy END method=%s client=%s target=%s status=%d durationMs=%d bytesOut=%d",
		r.Method, client, target, resp.StatusCode, elapsedMs(start), n)
}

func (h *handler) handleConnectCarrier(w http.ResponseWriter, r *http.Request, client, target string, start time.Time) {
	stream, err := h.parent.open()
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	outReq := r.Clone(r.Context())
	outReq.Header.Del("Proxy-Authorization")
	outReq.Header.Del("Proxy-Connection")
	if err := outReq.Write(stream); err != nil {
		_ = stream.Close()
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	br := bufio.NewReader(stream)
	resp, err := http.ReadResponse(br, outReq)
	if err != nil {
		_ = stream.Close()
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = stream.Close()
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = stream.Close()
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, buf, err := hj.Hijack()
	if err != nil {
		_ = stream.Close()
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if br.Buffered() > 0 {
		stream = &bufConn{Conn: stream, r: br}
	}
	if buf.Reader.Buffered() > 0 {
		_, _ = io.Copy(stream, buf)
	}
	in, out := relayCounted(clientConn, stream)
	h.log.Printf("proxy END method=CONNECT client=%s target=%s durationMs=%d bytesIn=%d bytesOut=%d",
		client, target, elapsedMs(start), in, out)
}

func (h *handler) handleForwardCarrier(w http.ResponseWriter, r *http.Request, client, target string, start time.Time) {
	stream, err := h.parent.open()
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer stream.Close()
	outReq := r.Clone(r.Context())
	outReq.Header.Del("Proxy-Authorization")
	outReq.Header.Del("Proxy-Connection")
	if err := outReq.WriteProxy(stream); err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(stream), outReq)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	h.log.Printf("proxy END method=%s client=%s target=%s status=%d durationMs=%d bytesOut=%d",
		r.Method, client, target, resp.StatusCode, elapsedMs(start), n)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func relayCounted(client, upstream net.Conn) (bytesIn, bytesOut int64) {
	defer client.Close()
	defer upstream.Close()
	var in, out atomic.Int64
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(upstream, client) // client → upstream
		in.Store(n)
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(client, upstream) // upstream → client
		out.Store(n)
		done <- struct{}{}
	}()
	<-done
	return in.Load(), out.Load()
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return net.ParseIP(remoteAddr)
	}
	return net.ParseIP(host)
}

func clientIPString(remoteAddr string) string {
	ip := remoteIP(remoteAddr)
	if ip == nil {
		host, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			return host
		}
		return remoteAddr
	}
	return ip.String()
}

func elapsedMs(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
