package main

import "testing"

func TestFormatBytes(t *testing.T) {
	if formatBytes(500) != "500 B" {
		t.Fatalf("got %q", formatBytes(500))
	}
	if formatBytes(1536) != "1.50 KiB" {
		t.Fatalf("got %q", formatBytes(1536))
	}
}

func TestFormatDuration(t *testing.T) {
	if formatDuration(0) != "0s" {
		t.Fatalf("got %q", formatDuration(0))
	}
}
