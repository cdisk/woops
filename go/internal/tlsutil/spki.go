package tlsutil

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// NormalizeSPKIPin accepts hex (64 chars) or base64 of SHA-256(SPKI DER),
// optionally prefixed with "sha256:" / "sha256/" / "sha256//".
// Returns lowercase hex. Empty input → empty pin (system CA mode).
func NormalizeSPKIPin(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	s = strings.TrimPrefix(s, "sha256://")
	s = strings.TrimPrefix(s, "sha256//")
	s = strings.TrimPrefix(s, "sha256:")
	s = strings.TrimPrefix(s, "sha256/")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty TLS SPKI pin")
	}
	if b, err := hex.DecodeString(s); err == nil && len(b) == sha256.Size {
		return hex.EncodeToString(b), nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && len(b) == sha256.Size {
			return hex.EncodeToString(b), nil
		}
	}
	return "", fmt.Errorf("TLS SPKI pin must be sha256 hex (64 chars) or base64 digest")
}

// SPKIPinHex returns lowercase hex SHA-256 of the certificate's SPKI.
func SPKIPinHex(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

// PinToCurlPinnedPubkey converts a pin to curl --pinnedpubkey value (sha256//base64).
func PinToCurlPinnedPubkey(pin string) (string, error) {
	norm, err := NormalizeSPKIPin(pin)
	if err != nil || norm == "" {
		return "", err
	}
	b, err := hex.DecodeString(norm)
	if err != nil {
		return "", err
	}
	return "sha256//" + base64.StdEncoding.EncodeToString(b), nil
}

// ClientTLSConfig returns TLS settings: system CA when pin empty; SPKI pin otherwise.
func ClientTLSConfig(pin string) (*tls.Config, error) {
	norm, err := NormalizeSPKIPin(pin)
	if err != nil {
		return nil, err
	}
	if norm == "" {
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}
	want := norm
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // pin check below replaces CA + hostname trust
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("tls: peer sent no certificates")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("tls: parse peer certificate: %w", err)
			}
			got := SPKIPinHex(cert)
			if !strings.EqualFold(got, want) {
				return fmt.Errorf("tls: SPKI pin mismatch")
			}
			return nil
		},
	}, nil
}
