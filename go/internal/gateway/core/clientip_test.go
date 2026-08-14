package core

import (
	"net/http"
	"testing"
)

func TestClientIPIgnoresXForwardedFor(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "171.213.155.199:54321",
		Header:     http.Header{"X-Forwarded-For": []string{"1.2.3.4, 5.6.7.8"}},
	}
	got := clientIP(r)
	if got != "171.213.155.199" {
		t.Fatalf("clientIP = %q, want RemoteAddr host (not XFF)", got)
	}
}

func TestClientIPWithoutPort(t *testing.T) {
	r := &http.Request{RemoteAddr: "10.0.0.1"}
	if got := clientIP(r); got != "10.0.0.1" {
		t.Fatalf("clientIP = %q", got)
	}
}
