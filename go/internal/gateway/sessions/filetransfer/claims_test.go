package filetransfer

import (
	"encoding/json"
	"testing"
)

func TestClaimsDecode(t *testing.T) {
	var claims Claims
	raw := []byte(`{"sessionId":"s","assetId":"a","type":"filetransfer","transferId":"t","direction":"upload","path":"/tmp/a","size":42,"fingerprint":"f","abort":true}`)
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.TransferID != "t" || claims.Path != "/tmp/a" || claims.Size != 42 || !claims.Abort {
		t.Fatalf("claims: %#v", claims)
	}
}
