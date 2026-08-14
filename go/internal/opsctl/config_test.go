package opsctl

import "testing"

func TestLoadConfigRequiresHTTPS(t *testing.T) {
	t.Setenv("OPSCTL_CONFIG", `{"server":"http://gw.example:9200","token":"ops_x_y","pin":""}`)
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected reject http")
	}
}

func TestLoadConfigOK(t *testing.T) {
	pin := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	t.Setenv("OPSCTL_CONFIG", `{"server":"https://gw.example:9200","token":"ops_x_y","pin":"`+pin+`"}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://gw.example:9200" || cfg.Token != "ops_x_y" || cfg.Pin != pin {
		t.Fatalf("%+v", cfg)
	}
	if cfg.http == nil || cfg.dialer == nil {
		t.Fatal("expected dialers")
	}
}
