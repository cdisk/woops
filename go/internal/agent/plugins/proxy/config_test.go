package proxy

import (
	"testing"
)

func TestParseAndValidate(t *testing.T) {
	raw := []byte(`
enabled: true
listen: "127.0.0.1:0"
username: u
password: p
allowCIDRs:
  - "10.0.0.0/8"
allowGlobal: true
`)
	cfg, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:0" || !cfg.AllowGlobal || len(cfg.cidrs) != 1 {
		t.Fatalf("%+v", cfg)
	}
}

func TestValidateRequiresAuth(t *testing.T) {
	cfg := Config{Enabled: true, Username: "u"}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateBadCIDR(t *testing.T) {
	cfg := Config{
		Enabled:    true,
		Username:   "u",
		Password:   "p",
		AllowCIDRs: []string{"not-a-cidr"},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDefaultListen(t *testing.T) {
	cfg := Config{Enabled: true, Username: "u", Password: "p"}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != defaultListen {
		t.Fatalf("listen %s", cfg.Listen)
	}
}

func TestExtractRaw(t *testing.T) {
	doc := []byte(`
server: "127.0.0.1:9200"
proxy:
  enabled: true
  username: u
  password: p
`)
	raw, err := ExtractRaw(doc)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.Username != "u" {
		t.Fatalf("%+v", cfg)
	}
}

func TestExtractRawMissing(t *testing.T) {
	raw, err := ExtractRaw([]byte("server: x\n"))
	if err != nil || raw != nil {
		t.Fatalf("got %q err=%v", raw, err)
	}
}
