package shell

import (
	"encoding/json"
	"testing"
)

func TestClaimsDecode(t *testing.T) {
	var claims Claims
	if err := json.Unmarshal([]byte(`{"sessionId":"s","assetId":"a","type":"shell","shellKind":"powershell"}`), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Type != "shell" || claims.ShellKind != "powershell" {
		t.Fatalf("claims: %#v", claims)
	}
}
