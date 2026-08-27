package proxy

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// opsTarget holds host + allowed ports derived from agent.yaml server.
type opsTarget struct {
	host  string
	ports map[int]struct{}
}

func parseOpsTarget(server string) (opsTarget, error) {
	s := strings.TrimSpace(server)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return opsTarget{}, fmt.Errorf("server empty")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return opsTarget{}, fmt.Errorf("bad server: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return opsTarget{}, fmt.Errorf("server missing host")
	}
	ports := map[int]struct{}{9100: {}}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return opsTarget{}, fmt.Errorf("server port: %w", err)
		}
		ports[n] = struct{}{}
	} else {
		switch strings.ToLower(u.Scheme) {
		case "https", "wss":
			ports[443] = struct{}{}
		default:
			ports[80] = struct{}{}
		}
	}
	return opsTarget{host: host, ports: ports}, nil
}

func (o opsTarget) allows(host string, port int) bool {
	if !strings.EqualFold(host, o.host) {
		return false
	}
	_, ok := o.ports[port]
	return ok
}

func (c *Config) clientAllowed(ip net.IP) bool {
	if len(c.cidrs) == 0 {
		return true
	}
	if ip == nil {
		return false
	}
	for _, n := range c.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// Alibaba Cloud instance metadata is outside link-local space.
		if ip4[0] == 100 && ip4[1] == 100 && ip4[2] == 100 && ip4[3] == 200 {
			return true
		}
	}
	if ip.Equal(net.ParseIP("fd00:ec2::254")) {
		return true
	}
	return false
}

// authorizeDest applies allowGlobal / ops-only and safety blocklist without DNS.
// Used when dialing via an upstream proxy (upstream resolves the name).
func (c *Config) authorizeDest(host string, port int, ops opsTarget) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("bad port %d", port)
	}

	opsOK := ops.allows(host, port)
	if !c.AllowGlobal && !opsOK {
		return fmt.Errorf("destination not allowed (ops host only)")
	}

	if ip := net.ParseIP(host); ip != nil {
		if !opsOK && isBlockedIP(ip) {
			return fmt.Errorf("destination blocked")
		}
		return nil
	}

	h := strings.ToLower(host)
	if !opsOK && (h == "localhost" || h == "localhost.localdomain") {
		return fmt.Errorf("destination blocked")
	}
	return nil
}

// resolveAndCheckDest authorizes then resolves hostname locally for direct dial.
// Ops-only destinations are exempt from the loopback blocklist so local Gateway works.
func (c *Config) resolveAndCheckDest(host string, port int, ops opsTarget) (string, error) {
	if err := c.authorizeDest(host, port, ops); err != nil {
		return "", err
	}
	host = strings.TrimSpace(host)

	if ip := net.ParseIP(host); ip != nil {
		return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
	}

	opsOK := ops.allows(host, port)
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("resolve: %w", err)
	}
	for _, ip := range ips {
		if !opsOK && isBlockedIP(ip) {
			continue
		}
		return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
	}
	return "", fmt.Errorf("destination blocked")
}

func sameProxyTarget(upstream *url.URL, host string, port int) bool {
	if upstream == nil {
		return false
	}
	uh := upstream.Hostname()
	up := upstream.Port()
	if up == "" {
		if strings.EqualFold(upstream.Scheme, "https") {
			up = "443"
		} else {
			up = "80"
		}
	}
	return strings.EqualFold(strings.TrimSpace(host), uh) && strconv.Itoa(port) == up
}

func splitHostPort(authority string, defaultPort int) (string, int, error) {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return "", 0, fmt.Errorf("empty authority")
	}
	if _, _, err := net.SplitHostPort(authority); err == nil {
		h, p, err := net.SplitHostPort(authority)
		if err != nil {
			return "", 0, err
		}
		port, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, err
		}
		return h, port, nil
	}
	// Bare IPv6 or hostname/IPv4 without port.
	if ip := net.ParseIP(strings.Trim(authority, "[]")); ip != nil {
		return ip.String(), defaultPort, nil
	}
	if strings.Contains(authority, ":") {
		// Ambiguous; try bracket form failure already happened.
		return "", 0, fmt.Errorf("bad authority %q", authority)
	}
	return authority, defaultPort, nil
}
