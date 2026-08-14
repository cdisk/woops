package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseHTTPProxy(t *testing.T) {
	u, err := ParseHTTPProxy("http://u:p@10.0.0.1:3128")
	if err != nil || u == nil || u.Host != "10.0.0.1:3128" {
		t.Fatalf("got %#v err %v", u, err)
	}
	if got := RedactProxyURL(u); !(strings.Contains(got, "u:") && strings.Contains(got, "@10.0.0.1:3128") && !strings.Contains(got, ":p@")) {
		t.Fatalf("redact should hide password: %q", got)
	}
	if _, err := ParseHTTPProxy("socks5://127.0.0.1:1080"); err == nil {
		t.Fatal("socks should fail")
	}
	u, err = ParseHTTPProxy("")
	if err != nil || u != nil {
		t.Fatalf("empty: %#v %v", u, err)
	}
}

func TestWSDialerExplicitProxy(t *testing.T) {
	d, err := WSDialer(DialOptions{
		Pin:   "",
		Proxy: "http://u:p@10.0.0.1:3128",
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://gw.example.com/", nil)
	pu, err := d.Proxy(req)
	if err != nil || pu == nil || pu.Host != "10.0.0.1:3128" {
		t.Fatalf("proxy %#v err %v", pu, err)
	}
}

func TestNormalizeSPKIPinRoundTrip(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	hexPin := hex.EncodeToString(raw)
	got, err := NormalizeSPKIPin("sha256:" + hexPin)
	if err != nil || got != hexPin {
		t.Fatalf("got %q err %v", got, err)
	}
	curl, err := PinToCurlPinnedPubkey(hexPin)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := NormalizeSPKIPin(curl)
	if err != nil || got2 != hexPin {
		t.Fatalf("curl roundtrip got %q", got2)
	}
}

func TestClientTLSConfigPin(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "gw"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pin := SPKIPinHex(cert)
	cfg, err := ClientTLSConfig(pin)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.VerifyPeerCertificate([][]byte{der}, nil); err != nil {
		t.Fatal(err)
	}
	bad, err := ClientTLSConfig("0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if err := bad.VerifyPeerCertificate([][]byte{der}, nil); err == nil {
		t.Fatal("expected mismatch")
	}
}
