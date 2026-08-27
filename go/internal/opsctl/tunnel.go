package opsctl

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type TunnelOptions struct {
	Protocol    string
	ListenHost  string
	ListenPort  int
	TargetHost  string
	TargetPort  int
	EphemeralID string
}

func ParseTunnelOptions(protocol, listen, target string) (TunnelOptions, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "tcp" && protocol != "udp" {
		return TunnelOptions{}, fmt.Errorf("protocol must be tcp or udp")
	}
	listenHost, listenPort, err := splitEndpoint(listen, true)
	if err != nil {
		return TunnelOptions{}, fmt.Errorf("listen: %w", err)
	}
	targetHost, targetPort, err := splitEndpoint(target, false)
	if err != nil {
		return TunnelOptions{}, fmt.Errorf("target: %w", err)
	}
	return TunnelOptions{
		Protocol: protocol, ListenHost: listenHost, ListenPort: listenPort,
		TargetHost: targetHost, TargetPort: targetPort,
	}, nil
}

func splitEndpoint(value string, defaultLoopback bool) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, fmt.Errorf("must be host:port (IPv6 must use [addr]:port): %w", err)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		if !defaultLoopback {
			return "", 0, fmt.Errorf("host is required")
		}
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return host, port, nil
}

func NewEphemeralID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return "opsctl:" + fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && (httpErr.StatusCode == 401 || httpErr.StatusCode == 403) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"token expired", "token revoked", "expired token", "revoked token",
		"invalid token", "does not allow", "permission denied", "forbidden", "unauthorized",
		"asset not found",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (o TunnelOptions) meta(clientAddr string) map[string]any {
	return map[string]any{
		"protocol": o.Protocol, "ephemeralId": o.EphemeralID,
		"listenHost": o.ListenHost, "listenPort": o.ListenPort,
		"targetHost": o.TargetHost, "targetPort": o.TargetPort,
		"clientAddr": clientAddr,
	}
}

func (c *Client) dialWS(ctx context.Context, rawURL string) (*websocket.Conn, error) {
	dialer := c.cfg.dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	ws, resp, err := dialer.DialContext(ctx, rawURL, nil)
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return nil, &HTTPError{StatusCode: resp.StatusCode, Body: err.Error()}
		}
		return nil, err
	}
	return ws, nil
}

func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	cap := attempt
	if cap > 5 {
		cap = 5
	}
	base := time.Second << cap
	jitter := 0.8 + mathrand.Float64()*0.4
	delay := time.Duration(float64(base) * jitter)
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func waitRetry(ctx context.Context, attempt int, logger *log.Logger, err error) bool {
	delay := retryDelay(attempt)
	if logger != nil {
		logger.Printf("temporary tunnel error: %v; retrying in %s", err, delay.Round(time.Millisecond))
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
