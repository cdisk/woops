package agentinstall

import "testing"

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		h    string
		want bool
	}{
		{"", false},
		{"identity", false},
		{"gzip", true},
		{"Gzip", true},
		{"deflate, gzip", true},
		{"gzip;q=1.0, identity;q=0.5", true},
		{"identity;q=1, *;q=0", false},
	}
	for _, c := range cases {
		if got := acceptsGzip(c.h); got != c.want {
			t.Fatalf("acceptsGzip(%q)=%v want %v", c.h, got, c.want)
		}
	}
}
