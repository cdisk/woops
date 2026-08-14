package core

import (
	"testing"
)

func TestWsBaseRejectsPlaintextRemote(t *testing.T) {
	if _, err := wsBase("http://evil.example:9200"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := wsBase("evil.example:9200"); err == nil {
		t.Fatal("expected reject bare remote")
	}
	got, err := wsBase("https://gw.example.com")
	if err != nil || got != "wss://gw.example.com" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = wsBase("127.0.0.1:9200")
	if err != nil || got != "ws://127.0.0.1:9200" {
		t.Fatalf("loopback bare: got %q err %v", got, err)
	}
}
