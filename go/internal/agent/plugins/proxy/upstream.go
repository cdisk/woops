package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func proxyDialAddr(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port)
}

func sameListenAddress(upstream *url.URL, listen string) bool {
	if upstream == nil {
		return false
	}
	lh, lp, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return false
	}
	uh, up, err := net.SplitHostPort(proxyDialAddr(upstream))
	if err != nil || lp != up {
		return false
	}
	if strings.EqualFold(lh, uh) {
		return true
	}
	if strings.EqualFold(lh, "localhost") {
		lh = "127.0.0.1"
	}
	if strings.EqualFold(uh, "localhost") {
		uh = "127.0.0.1"
	}
	lip, uip := net.ParseIP(lh), net.ParseIP(uh)
	if lip != nil && lip.IsUnspecified() && uip != nil && uip.IsLoopback() {
		return true
	}
	return lip != nil && uip != nil && lip.IsLoopback() && uip.IsLoopback()
}

// dialViaUpstreamCONNECT opens TCP to upstream HTTP proxy and issues CONNECT to dest.
func dialViaUpstreamCONNECT(upstream *url.URL, destHost string, destPort int) (net.Conn, error) {
	if upstream == nil {
		return nil, fmt.Errorf("upstream nil")
	}
	dest := net.JoinHostPort(destHost, strconv.Itoa(destPort))
	conn, err := net.DialTimeout("tcp", proxyDialAddr(upstream), dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial upstream: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))

	var b strings.Builder
	b.WriteString("CONNECT ")
	b.WriteString(dest)
	b.WriteString(" HTTP/1.1\r\nHost: ")
	b.WriteString(dest)
	b.WriteString("\r\n")
	if upstream.User != nil {
		user := upstream.User.Username()
		pass, _ := upstream.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		b.WriteString("Proxy-Authorization: Basic ")
		b.WriteString(token)
		b.WriteString("\r\n")
	}
	b.WriteString("Proxy-Connection: Keep-Alive\r\n\r\n")
	if _, err := conn.Write([]byte(b.String())); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("upstream CONNECT write: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("upstream CONNECT read: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("upstream CONNECT status %d", resp.StatusCode)
	}
	// Buffered bytes (if any) belong to the tunnel; wrap so relay does not drop them.
	if br.Buffered() > 0 {
		conn = &bufConn{Conn: conn, r: br}
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}
