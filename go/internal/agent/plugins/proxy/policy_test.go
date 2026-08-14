package proxy

import (
	"net"
	"net/url"
	"testing"
)

func TestParseOpsTarget(t *testing.T) {
	ops, err := parseOpsTarget("127.0.0.1:9200")
	if err != nil {
		t.Fatal(err)
	}
	if !ops.allows("127.0.0.1", 9200) || !ops.allows("127.0.0.1", 9100) {
		t.Fatalf("ports %+v", ops.ports)
	}
	if ops.allows("127.0.0.1", 9090) || ops.allows("example.com", 9200) {
		t.Fatal("should deny")
	}
}

func TestClientAllowed(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		Username:   "u",
		Password:   "p",
		AllowCIDRs: []string{"10.0.0.0/8"},
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if !cfg.clientAllowed(net.ParseIP("10.1.2.3")) {
		t.Fatal("10.x should allow")
	}
	if cfg.clientAllowed(net.ParseIP("192.168.1.1")) {
		t.Fatal("192.168 should deny")
	}
}

func TestResolveOpsOnly(t *testing.T) {
	cfg := Config{AllowGlobal: false}
	ops, _ := parseOpsTarget("10.0.0.1:9200")
	dest, err := cfg.resolveAndCheckDest("10.0.0.1", 9200, ops)
	if err != nil {
		t.Fatal(err)
	}
	if dest != "10.0.0.1:9200" {
		t.Fatalf("dest %s", dest)
	}
	if _, err := cfg.resolveAndCheckDest("8.8.8.8", 443, ops); err == nil {
		t.Fatal("expected deny")
	}
}

func TestResolveBlockLoopbackWhenGlobal(t *testing.T) {
	cfg := Config{AllowGlobal: true}
	ops, _ := parseOpsTarget("10.0.0.1:9200")
	if _, err := cfg.resolveAndCheckDest("127.0.0.1", 9100, ops); err == nil {
		t.Fatal("loopback should be blocked when global")
	}
	if _, err := cfg.resolveAndCheckDest("169.254.169.254", 80, ops); err == nil {
		t.Fatal("metadata should be blocked")
	}
}

func TestResolveOpsLoopbackExempt(t *testing.T) {
	cfg := Config{AllowGlobal: false}
	ops, _ := parseOpsTarget("127.0.0.1:9200")
	if _, err := cfg.resolveAndCheckDest("127.0.0.1", 9200, ops); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeDestNoDNS(t *testing.T) {
	cfg := Config{AllowGlobal: false}
	ops, _ := parseOpsTarget("https://liteops.example:9200")
	if err := cfg.authorizeDest("liteops.example", 9200, ops); err != nil {
		t.Fatal(err)
	}
	if err := cfg.authorizeDest("evil.example", 443, ops); err == nil {
		t.Fatal("expected deny")
	}
}

func TestSameProxyTarget(t *testing.T) {
	u, _ := url.Parse("http://10.0.0.1:3128")
	if !sameProxyTarget(u, "10.0.0.1", 3128) {
		t.Fatal("should detect loop")
	}
	if sameProxyTarget(u, "liteops.example", 9200) {
		t.Fatal("gateway is not upstream")
	}
}
