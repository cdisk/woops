package core

import (
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/tlsutil"
)

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// NewWSDialer validates gateway URL shape, then builds the shared tlsutil dialer
// (SPKI pin + optional gatewayProxy). All Agent→Gateway WSS reuse this dialer.
func NewWSDialer(server, pin, gatewayProxy string) (*websocket.Dialer, error) {
	base, err := wsBase(server)
	if err != nil {
		return nil, err
	}
	if _, err := url.Parse(base); err != nil {
		return nil, err
	}
	opts := tlsutil.DialOptions{Pin: pin, Proxy: gatewayProxy}
	if u, err := tlsutil.ParseHTTPProxy(opts.Proxy); err != nil {
		return nil, err
	} else if u != nil {
		log.Printf("gateway WSS via proxy %s", tlsutil.RedactProxyURL(u))
	}
	return tlsutil.WSDialer(opts, 45*time.Second)
}
