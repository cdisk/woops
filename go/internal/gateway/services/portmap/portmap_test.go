package portmap

import "testing"

func TestNormalizeDirection(t *testing.T) {
	if normalizeDirection("") != dirGatewayToAsset {
		t.Fatal("empty")
	}
	if normalizeDirection("reverse") != dirAssetToGateway {
		t.Fatal("reverse")
	}
	if normalizeDirection("asset_to_gateway") != dirAssetToGateway {
		t.Fatal("asset_to_gateway")
	}
	if normalizeDirection("forward") != dirGatewayToAsset {
		t.Fatal("forward")
	}
}

func TestListeningPortsSkipsReverse(t *testing.T) {
	m := NewManager()
	m.byID["f"] = &entry{runtime: runtime{
		MappingID: "f", Direction: dirGatewayToAsset, ListenPort: 20001, Listening: true,
	}}
	m.byID["r"] = &entry{runtime: runtime{
		MappingID: "r", Direction: dirAssetToGateway, ListenPort: 20002, Listening: true,
	}}
	ports := m.listeningPorts()
	if len(ports) != 1 || ports[0] != 20001 {
		t.Fatalf("ports=%v", ports)
	}
}

func TestAuditDetailIncludesDirection(t *testing.T) {
	e := &entry{runtime: runtime{
		MappingID: "m", Direction: dirAssetToGateway,
		ListenHost: "127.0.0.1", ListenPort: 3306,
		TargetHost: "db.internal", TargetPort: 5432,
	}}
	detail := e.auditDetail("127.0.0.1:9999")
	if detail["direction"] != dirAssetToGateway {
		t.Fatalf("%v", detail)
	}
	if detail["listenHost"] != "127.0.0.1" || detail["clientAddr"] != "127.0.0.1:9999" {
		t.Fatalf("%v", detail)
	}
}
