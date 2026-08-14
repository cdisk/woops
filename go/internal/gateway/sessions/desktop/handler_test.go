package desktop

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ops-bastion/ops/go/internal/gateway/sessions/desktop/guac"
)

type captureTransport struct{ messages [][]byte }

func (w *captureTransport) write(data []byte) error {
	w.messages = append(w.messages, append([]byte(nil), data...))
	return nil
}

func TestForwardBrowserInstructions(t *testing.T) {
	ping := guac.Encode("", "ping", "12345")
	mouse := guac.Encode("mouse", "10", "20", "0")
	var browser captureTransport
	var guacd bytes.Buffer
	if err := forwardBrowserInstructions(append(ping, mouse...), &browser, &guacd); err != nil {
		t.Fatal(err)
	}
	if len(browser.messages) != 1 || !bytes.Equal(browser.messages[0], ping) || !bytes.Equal(guacd.Bytes(), mouse) {
		t.Fatalf("unexpected relay browser=%q guacd=%q", browser.messages, guacd.Bytes())
	}
	if err := forwardBrowserInstructions(mouse[:len(mouse)-1], &browser, &guacd); err == nil {
		t.Fatal("expected partial instruction error")
	}
}

func TestClaimsDecode(t *testing.T) {
	raw := []byte(`{"sessionId":"s","assetId":"a","type":"rdp","targetHost":"127.0.0.1","targetPort":3389,"desktopUsername":"admin"}`)
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Type != "rdp" || claims.TargetPort != 3389 || claims.DesktopUsername != "admin" {
		t.Fatalf("claims: %#v", claims)
	}
}
