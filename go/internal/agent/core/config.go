package core

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ops-bastion/ops/go/internal/tlsutil"
	"gopkg.in/yaml.v3"
)

// Config splits operator settings from server-issued credentials.
//
//	/etc/woops-agent/agent.yaml   -> gateway (https://…), optional gatewayTlsSpkiSha256
//	/etc/woops-agent/asset-id     -> assets.id issued at register
//	/etc/woops-agent/agent-token  -> issued / refreshed at register
type Config struct {
	Gateway              string
	GatewayTLSSpkiSHA256 string
	// GatewayProxy is optional HTTP(S) proxy for all Gateway WSS (control/session/metrics).
	// Distinct from proxy: (inbound forward-proxy plugin).
	GatewayProxy    string
	AssetID         string
	AgentToken      string
	ControlWS       string
	SessionWS       string
	MetricsWS       string
	MetricsEnabled  bool
	MetricsInterval time.Duration
	// ProxyRaw is the optional proxy: YAML subtree for the proxy plugin (soft-fail).
	ProxyRaw []byte
	// ConfigPath is the agent.yaml path (for local log file placement).
	ConfigPath string
}

type fileConfig struct {
	Gateway              string      `yaml:"gateway"`
	GatewayTLSSpkiSHA256 string      `yaml:"gatewayTlsSpkiSha256"`
	GatewayProxy         string      `yaml:"gatewayProxy"`
	Metrics              metricsYAML `yaml:"metrics"`
}

type metricsYAML struct {
	Enabled     *bool `yaml:"enabled"`
	IntervalSec int   `yaml:"intervalSec"`
}

// LoadConfig reads agent.yaml (gateway) plus asset-id / agent-token beside it.
// Optional proxy: subtree is extracted for the plugin; extract/parse errors do not fail LoadConfig.
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var fc fileConfig
	if err := yaml.Unmarshal(b, &fc); err != nil {
		return Config{}, err
	}
	dir := filepath.Dir(path)
	cfg := Config{
		Gateway:              strings.TrimSpace(fc.Gateway),
		GatewayTLSSpkiSHA256: strings.TrimSpace(fc.GatewayTLSSpkiSHA256),
		GatewayProxy:         strings.TrimSpace(fc.GatewayProxy),
		AssetID:              strings.TrimSpace(readFile(filepath.Join(dir, "asset-id"))),
		AgentToken:           strings.TrimSpace(readFile(filepath.Join(dir, "agent-token"))),
		ConfigPath:           path,
	}
	cfg.MetricsEnabled = true
	if fc.Metrics.Enabled != nil {
		cfg.MetricsEnabled = *fc.Metrics.Enabled
	}
	if fc.Metrics.IntervalSec > 0 {
		cfg.MetricsInterval = time.Duration(fc.Metrics.IntervalSec) * time.Second
	} else {
		cfg.MetricsInterval = 60 * time.Second
	}
	if raw, err := extractMapping(b, "proxy"); err != nil {
		log.Printf("proxy plugin: extract config: %v", err)
	} else {
		cfg.ProxyRaw = raw
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func extractMapping(doc []byte, key string) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, err
	}
	node := &root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key || node.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		return yaml.Marshal(node.Content[i+1])
	}
	return nil, nil
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// ReloadCredentials re-reads asset-id / agent-token beside agent.yaml.
// Used before control/metrics reconnect so online updates that rotate the token
// do not leave the process stuck on a stale in-memory credential.
func (c *Config) ReloadCredentials() error {
	if strings.TrimSpace(c.ConfigPath) == "" {
		return fmt.Errorf("config path unset")
	}
	dir := filepath.Dir(c.ConfigPath)
	id := strings.TrimSpace(readFile(filepath.Join(dir, "asset-id")))
	tok := strings.TrimSpace(readFile(filepath.Join(dir, "agent-token")))
	if id == "" || tok == "" {
		return fmt.Errorf("missing asset-id or agent-token beside %s", c.ConfigPath)
	}
	c.AssetID = id
	c.AgentToken = tok
	return nil
}

func (c *Config) normalize() error {
	if c.Gateway == "" {
		return fmt.Errorf("gateway required in agent.yaml")
	}
	if c.AssetID == "" || c.AgentToken == "" {
		return fmt.Errorf("missing asset-id or agent-token beside agent.yaml")
	}
	pin, err := tlsutil.NormalizeSPKIPin(c.GatewayTLSSpkiSHA256)
	if err != nil {
		return err
	}
	c.GatewayTLSSpkiSHA256 = pin
	if _, err := tlsutil.ParseHTTPProxy(c.GatewayProxy); err != nil {
		return err
	}
	base, err := wsBase(c.Gateway)
	if err != nil {
		return err
	}
	c.ControlWS = base + "/ws/agent/control"
	c.SessionWS = base + "/ws/agent/session"
	c.MetricsWS = base + "/ws/agent/metrics"
	return nil
}

func wsBase(gateway string) (string, error) {
	s := strings.TrimSpace(gateway)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "", fmt.Errorf("gateway empty")
	}
	if !strings.Contains(s, "://") {
		// Bare host:port - only loopback may use plaintext ws:// (local dev).
		host := s
		if h, _, err := net.SplitHostPort(s); err == nil {
			host = h
		}
		if !isLoopbackHost(host) {
			return "", fmt.Errorf("gateway %q needs https:// (or wss://); bare host only allowed for localhost", gateway)
		}
		s = "ws://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("bad gateway: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported gateway scheme %q", u.Scheme)
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	if u.Host == "" {
		return "", fmt.Errorf("gateway missing host")
	}
	if u.Scheme == "ws" && !isLoopbackHost(u.Hostname()) {
		return "", fmt.Errorf("plaintext ws:// to %s forbidden; use https:// and TLS", u.Hostname())
	}
	return u.String(), nil
}

func gatewayHTTPBase(gateway string) (string, error) {
	s := strings.TrimSpace(gateway)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "", fmt.Errorf("gateway empty")
	}
	if !strings.Contains(s, "://") {
		host := s
		if h, _, err := net.SplitHostPort(s); err == nil {
			host = h
		}
		if isLoopbackHost(host) {
			return "http://" + s, nil
		}
		return "https://" + s, nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("bad gateway: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported gateway scheme %q", u.Scheme)
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
