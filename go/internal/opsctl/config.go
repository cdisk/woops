package opsctl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ops-bastion/ops/go/internal/tlsutil"
)

// Config is loaded from OPSCTL_CONFIG JSON:
//
//	{"server":"https://gw:9200","token":"ops_…","pin":"<hex>"}
type Config struct {
	Server string
	Token  string
	Pin    string

	http   *http.Client
	dialer *websocket.Dialer
}

type fileConfig struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	Pin    string `json:"pin"`
}

func LoadConfig() (Config, error) {
	raw := strings.TrimSpace(os.Getenv("OPSCTL_CONFIG"))
	if raw == "" {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG is required (JSON with server, token, pin)")
	}
	var fc fileConfig
	if err := json.Unmarshal([]byte(raw), &fc); err != nil {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG: invalid JSON: %w", err)
	}
	cfg := Config{
		Server: strings.TrimRight(strings.TrimSpace(fc.Server), "/"),
		Token:  strings.TrimSpace(fc.Token),
		Pin:    strings.TrimSpace(fc.Pin),
	}
	if cfg.Server == "" {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG.server is required (https Gateway base URL)")
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG.token is required")
	}
	u, err := url.Parse(cfg.Server)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG.server must be an absolute URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG.server must use https:// (got %s)", u.Scheme)
	}
	pin, err := tlsutil.NormalizeSPKIPin(cfg.Pin)
	if err != nil {
		return Config{}, fmt.Errorf("OPSCTL_CONFIG.pin: %w", err)
	}
	cfg.Pin = pin

	opts := tlsutil.DialOptions{Pin: cfg.Pin} // proxy: process env if set
	httpClient, err := tlsutil.HTTPClient(opts, 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	dialer, err := tlsutil.WSDialer(opts, 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.http = httpClient
	cfg.dialer = dialer
	return cfg, nil
}
