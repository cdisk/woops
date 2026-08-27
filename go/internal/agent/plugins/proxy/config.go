package proxy

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultListen = "0.0.0.0:3128"

// Config is the operator-facing proxy plugin settings.
type Config struct {
	Enabled     bool     `yaml:"enabled"`
	Listen      string   `yaml:"listen"`
	Username    string   `yaml:"username"`
	Password    string   `yaml:"password"`
	AllowCIDRs  []string `yaml:"allowCIDRs"`
	AllowGlobal bool     `yaml:"allowGlobal"`

	// Parsed at validate time.
	cidrs []*net.IPNet
}

type BridgeTarget struct {
	Address string `yaml:"address"`
	Key     string `yaml:"key"`

	keyBytes []byte
}

// BridgeConfig is independent from Config: listeners and outbound carriers may
// be enabled without exposing the ordinary HTTP proxy listener.
type BridgeConfig struct {
	Enabled     bool           `yaml:"enabled"`
	Listen      string         `yaml:"listen"`
	Key         string         `yaml:"key"`
	AllowGlobal bool           `yaml:"allowGlobal"`
	Targets     []BridgeTarget `yaml:"targets"`

	keyBytes []byte
}

func parse(raw []byte) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse: %w", err)
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	return cfg, nil
}

func parseBridge(raw []byte) (BridgeConfig, error) {
	if len(raw) == 0 {
		return BridgeConfig{}, nil
	}
	var cfg BridgeConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return BridgeConfig{}, fmt.Errorf("parse: %w", err)
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.Key = strings.TrimSpace(cfg.Key)
	for i := range cfg.Targets {
		cfg.Targets[i].Address = strings.TrimSpace(cfg.Targets[i].Address)
		cfg.Targets[i].Key = strings.TrimSpace(cfg.Targets[i].Key)
	}
	return cfg, nil
}

func decodeBridgeKey(value string) ([]byte, error) {
	if len(value) != 64 {
		return nil, fmt.Errorf("key must be exactly 64 hexadecimal characters")
	}
	key, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("key must be exactly 64 hexadecimal characters")
	}
	return key, nil
}

func (c *BridgeConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	var err error
	listenKey := ""
	if c.Listen != "" {
		c.keyBytes, err = decodeBridgeKey(c.Key)
		if err != nil {
			return fmt.Errorf("listener %w", err)
		}
		if _, err := net.ResolveTCPAddr("tcp", c.Listen); err != nil {
			return fmt.Errorf("listen %q: %w", c.Listen, err)
		}
		listenKey, _ = canonicalBridgeAddress(c.Listen)
	}
	seen := make(map[string]struct{}, len(c.Targets))
	for i := range c.Targets {
		target := &c.Targets[i]
		if target.Address == "" {
			return fmt.Errorf("targets[%d].address required", i)
		}
		if _, err := net.ResolveTCPAddr("tcp", target.Address); err != nil {
			return fmt.Errorf("targets[%d].address %q: %w", i, target.Address, err)
		}
		targetKey, _ := canonicalBridgeAddress(target.Address)
		if _, exists := seen[targetKey]; exists {
			return fmt.Errorf("targets[%d].address %q duplicated", i, target.Address)
		}
		seen[targetKey] = struct{}{}
		if listenKey != "" && bridgeAddressesOverlap(listenKey, targetKey) {
			return fmt.Errorf("targets[%d].address %q points to bridge listen", i, target.Address)
		}
		target.keyBytes, err = decodeBridgeKey(target.Key)
		if err != nil {
			return fmt.Errorf("targets[%d] %w", i, err)
		}
	}
	if c.Listen == "" && len(c.Targets) == 0 {
		return fmt.Errorf("listen or targets required when enabled")
	}
	return nil
}

func canonicalBridgeAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", err
	}
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "localhost" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

func bridgeAddressesOverlap(left, right string) bool {
	leftHost, leftPort, leftErr := net.SplitHostPort(left)
	rightHost, rightPort, rightErr := net.SplitHostPort(right)
	if leftErr != nil || rightErr != nil || leftPort != rightPort {
		return false
	}
	if strings.EqualFold(leftHost, rightHost) {
		return true
	}
	leftIP, rightIP := net.ParseIP(leftHost), net.ParseIP(rightHost)
	return leftIP != nil && rightIP != nil &&
		((leftIP.IsUnspecified() && rightIP.IsLoopback()) ||
			(rightIP.IsUnspecified() && leftIP.IsLoopback()))
}

func (c *Config) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("username and password required when enabled (uncomment/set proxy.username and proxy.password)")
	}
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	if _, err := net.ResolveTCPAddr("tcp", c.Listen); err != nil {
		return fmt.Errorf("listen %q: %w", c.Listen, err)
	}
	c.cidrs = c.cidrs[:0]
	for _, s := range c.AllowCIDRs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return fmt.Errorf("allowCIDRs %q: %w", s, err)
		}
		c.cidrs = append(c.cidrs, n)
	}
	return nil
}
