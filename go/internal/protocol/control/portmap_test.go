package control

import (
	"encoding/json"
	"testing"
)

func TestPortmapListenRoundTrip(t *testing.T) {
	raw, err := Marshal("portmap_listen", "m1", PortmapListenPayload{
		MappingID:  "m1",
		Protocol:   "tcp",
		ListenHost: "127.0.0.1",
		ListenPort: 22000,
		TargetHost: "10.0.0.1",
		TargetPort: 5432,
	})
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != "portmap_listen" || env.Version != ProtocolVersion {
		t.Fatalf("env %+v", env)
	}
	p, err := UnmarshalPayload[PortmapListenPayload](env)
	if err != nil {
		t.Fatal(err)
	}
	if p.MappingID != "m1" || p.ListenPort != 22000 || p.Protocol != "tcp" {
		t.Fatalf("payload %+v", p)
	}
}

func TestPortmapListenStatusRoundTrip(t *testing.T) {
	raw, err := Marshal("portmap_listen_status", "m2", PortmapListenStatusPayload{
		MappingID: "m2", OK: false, Error: "bind: address already in use",
	})
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	p, err := UnmarshalPayload[PortmapListenStatusPayload](env)
	if err != nil {
		t.Fatal(err)
	}
	if p.OK || p.Error == "" {
		t.Fatalf("payload %+v", p)
	}
}

func TestUnknownPayloadIgnoredShape(t *testing.T) {
	// Unknown types are ignored by receivers; ensure marshal still produces Envelope.
	raw, err := Marshal("portmap_unlisten", "", PortmapUnlistenPayload{MappingID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != "portmap_unlisten" {
		t.Fatalf("type %s", env.Type)
	}
}
