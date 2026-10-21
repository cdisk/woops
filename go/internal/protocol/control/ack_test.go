package control

import (
	"encoding/json"
	"testing"
)

func TestAckPayloadPingSecondsOptional(t *testing.T) {
	var with AckPayload
	if err := json.Unmarshal([]byte(`{"ok":true,"pingSeconds":5}`), &with); err != nil {
		t.Fatal(err)
	}
	if !with.OK || with.PingSeconds != 5 {
		t.Fatalf("%+v", with)
	}
	var without AckPayload
	if err := json.Unmarshal([]byte(`{"ok":true}`), &without); err != nil {
		t.Fatal(err)
	}
	if !without.OK || without.PingSeconds != 0 {
		t.Fatalf("%+v", without)
	}
	raw, err := json.Marshal(AckPayload{OK: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("old agents must ignore an absent field, got %s", raw)
	}
}
