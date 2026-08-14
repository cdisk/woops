package tlsutil

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DialOptions is the shared outbound policy for HTTPS / WSS clients
// (Agent Gateway links, opsctl, etc.): SPKI pin + optional HTTP proxy.
type DialOptions struct {
	// Pin is lowercase hex SPKI SHA-256, or empty for system CA trust.
	Pin string
	// Proxy is an optional http:// or https:// proxy URL (user:pass@host:port).
	// Empty means use process env (HTTPS_PROXY / HTTP_PROXY / ALL_PROXY).
	Proxy string
}

// ParseHTTPProxy validates an optional HTTP(S) proxy URL.
// Empty input returns (nil, nil) - callers then use ProxyFromEnvironment.
func ParseHTTPProxy(raw string) (*url.URL, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("proxy URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("proxy URL: scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("proxy URL: missing host")
	}
	return u, nil
}

// RedactProxyURL masks the password for logs.
func RedactProxyURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	if c.User != nil {
		user := c.User.Username()
		if _, has := c.User.Password(); has {
			c.User = url.UserPassword(user, "***")
		} else {
			c.User = url.User(user)
		}
	}
	return c.String()
}

func (o DialOptions) proxyFunc() (func(*http.Request) (*url.URL, error), *url.URL, error) {
	u, err := ParseHTTPProxy(o.Proxy)
	if err != nil {
		return nil, nil, err
	}
	if u != nil {
		return http.ProxyURL(u), u, nil
	}
	return http.ProxyFromEnvironment, nil, nil
}

// HTTPClient builds an HTTP client with DialOptions (pin + optional proxy).
func HTTPClient(o DialOptions, timeout time.Duration) (*http.Client, error) {
	tlsCfg, err := ClientTLSConfig(o.Pin)
	if err != nil {
		return nil, err
	}
	proxy, _, err := o.proxyFunc()
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:           proxy,
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

// WSDialer builds a WebSocket dialer with DialOptions (pin + optional proxy).
// Explicit Proxy uses HTTP CONNECT for wss (target DNS is resolved by the proxy).
func WSDialer(o DialOptions, handshakeTimeout time.Duration) (*websocket.Dialer, error) {
	tlsCfg, err := ClientTLSConfig(o.Pin)
	if err != nil {
		return nil, err
	}
	proxy, _, err := o.proxyFunc()
	if err != nil {
		return nil, err
	}
	return &websocket.Dialer{
		Proxy:            proxy,
		HandshakeTimeout: handshakeTimeout,
		TLSClientConfig:  tlsCfg,
	}, nil
}
