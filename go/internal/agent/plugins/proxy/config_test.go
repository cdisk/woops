package proxy

import (
	"strings"
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

func TestParseBridgeIndependentAndValidatesKeys(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cfg, err := parseBridge([]byte(`
enabled: true
listen: "127.0.0.1:3130"
key: "` + key + `"
allowGlobal: true
targets:
  - address: "10.0.0.2:3130"
    key: "` + key + `"
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || !cfg.AllowGlobal || len(cfg.keyBytes) != 32 || len(cfg.Targets[0].keyBytes) != 32 {
		t.Fatalf("%+v", cfg)
	}
}

func TestBridgeKeyMustBe64Hex(t *testing.T) {
	for _, key := range []string{"short", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		cfg := BridgeConfig{Enabled: true, Listen: "127.0.0.1:3130", Key: key}
		if err := cfg.validate(); err == nil {
			t.Fatalf("key %q should fail", key)
		}
	}
}

func TestBridgeTargetsWorkWithoutListen(t *testing.T) {
	key := strings.Repeat("ab", 32)
	cfg := BridgeConfig{
		Enabled: true,
		Targets: []BridgeTarget{{Address: "127.0.0.1:3130", Key: key}},
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeRejectsDuplicateAndSelfTargets(t *testing.T) {
	key := strings.Repeat("ab", 32)
	duplicate := BridgeConfig{
		Enabled: true,
		Targets: []BridgeTarget{
			{Address: "127.0.0.1:3999", Key: key},
			{Address: "localhost:3999", Key: key},
		},
	}
	if err := duplicate.validate(); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate error = %v", err)
	}

	self := BridgeConfig{
		Enabled: true,
		Listen:  "0.0.0.0:3999",
		Key:     key,
		Targets: []BridgeTarget{{Address: "127.0.0.1:3999", Key: key}},
	}
	if err := self.validate(); err == nil || !strings.Contains(err.Error(), "points to bridge listen") {
		t.Fatalf("self target error = %v", err)
	}
}
