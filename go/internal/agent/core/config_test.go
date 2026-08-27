package core

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayHTTPBase(t *testing.T) {
	cases := map[string]string{
		"https://gw.example:9200": "https://gw.example:9200",
		"wss://gw.example:9200":   "https://gw.example:9200",
		"http://127.0.0.1:9200":   "http://127.0.0.1:9200",
		"ws://127.0.0.1:9200":     "http://127.0.0.1:9200",
	}
	for in, want := range cases {
		got, err := gatewayHTTPBase(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestWsBase(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:9200":         "ws://127.0.0.1:9200",
		"http://127.0.0.1:9200":  "ws://127.0.0.1:9200",
		"https://gw.example.com": "wss://gw.example.com",
		"ws://127.0.0.1:9200/":   "ws://127.0.0.1:9200",
	}
	for in, want := range cases {
		got, err := wsBase(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
	if _, err := wsBase("10.0.0.1:9200"); err == nil {
		t.Fatal("bare remote host should require https://")
	}
	if _, err := wsBase("ws://evil.example:9200"); err == nil {
		t.Fatal("plaintext remote ws should be rejected")
	}
}

func TestLoadConfigCredFiles(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(yamlPath, []byte("gateway: 127.0.0.1:9200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset-id"), []byte("id-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AssetID != "id-1" || cfg.AgentToken != "tok-1" {
		t.Fatalf("creds %+v", cfg)
	}
	if cfg.ControlWS != "ws://127.0.0.1:9200/ws/agent/control" {
		t.Fatalf("ws %s", cfg.ControlWS)
	}
	if cfg.MetricsWS != "ws://127.0.0.1:9200/ws/agent/metrics" {
		t.Fatalf("metrics ws %s", cfg.MetricsWS)
	}
	if !cfg.MetricsEnabled {
		t.Fatal("metrics should default enabled")
	}
}

func TestLoadConfigAllowsMissingCredentials(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(yamlPath, []byte("gateway: 127.0.0.1:9200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AssetID != "" || cfg.AgentToken != "" {
		t.Fatalf("unexpected credentials asset=%q token=%q", cfg.AssetID, cfg.AgentToken)
	}
}

func TestReloadCredentials(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(yamlPath, []byte("gateway: 127.0.0.1:9200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset-id"), []byte("id-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ReloadCredentials(); err != nil {
		t.Fatal(err)
	}
	if cfg.AgentToken != "tok-new" || cfg.AssetID != "id-1" {
		t.Fatalf("creds asset=%q token=%q", cfg.AssetID, cfg.AgentToken)
	}
}

func TestLoadConfigGatewayProxy(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	content := `
gateway: https://gw.example.com:9200
gatewayProxy: "http://u:p@10.0.0.1:3128"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset-id"), []byte("id-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GatewayProxy != "http://u:p@10.0.0.1:3128" {
		t.Fatalf("gatewayProxy %q", cfg.GatewayProxy)
	}
	d, err := NewWSDialer(cfg.Gateway, cfg.GatewayTLSSpkiSHA256, cfg.GatewayProxy)
	if err != nil {
		t.Fatal(err)
	}
	if d.Proxy == nil {
		t.Fatal("expected Proxy set")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://gw.example.com:9200/", nil)
	pu, err := d.Proxy(req)
	if err != nil || pu == nil || pu.Host != "10.0.0.1:3128" {
		t.Fatalf("proxy URL %#v err %v", pu, err)
	}
}

func TestLoadConfigBadProxyDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	content := `
gateway: 127.0.0.1:9200
proxy:
  enabled: true
  username: only-user
  allowCIDRs:
    - "not-a-cidr"
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset-id"), []byte("id-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatalf("core LoadConfig must succeed: %v", err)
	}
	if len(cfg.ProxyRaw) == 0 {
		t.Fatal("expected ProxyRaw extracted for plugin soft-fail")
	}
	if cfg.ControlWS == "" {
		t.Fatal("core ws paths required")
	}
}

func TestLoadConfigExtractsIndependentProxyBridge(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	content := `
gateway: 127.0.0.1:9200
proxy:
  enabled: false
proxyBridge:
  enabled: true
  targets:
    - address: 10.0.0.2:3130
      key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`
	if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset-id"), []byte("id-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-token"), []byte("tok-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProxyRaw) == 0 || len(cfg.ProxyBridgeRaw) == 0 {
		t.Fatalf("proxy=%q bridge=%q", cfg.ProxyRaw, cfg.ProxyBridgeRaw)
	}
}
