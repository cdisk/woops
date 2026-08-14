package proxy

import (
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
