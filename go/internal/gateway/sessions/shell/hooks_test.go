package shell

import "testing"

func TestParseResize(t *testing.T) {
	cols, rows, ok := ParseResize([]byte("R,120,40"))
	if !ok || cols != 120 || rows != 40 {
		t.Fatalf("ParseResize = %d %d %v", cols, rows, ok)
	}
	if _, _, ok := ParseResize([]byte("hello")); ok {
		t.Fatal("ParseResize accepted non-control frame")
	}
	if _, _, ok := ParseResize([]byte("R,0,0")); ok {
		t.Fatal("ParseResize accepted zero geometry")
	}
}

func TestCastTitle(t *testing.T) {
	if got := CastTitle("bash", "alice", "asset-1"); got != "bash alice@asset-1" {
		t.Fatalf("got %q", got)
	}
	if got := CastTitle("", "", "asset-1"); got != "shell asset-1" {
		t.Fatalf("got %q", got)
	}
}
