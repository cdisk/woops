package netiface

import "testing"

func TestIsVirtualName(t *testing.T) {
	virtual := []string{
		"lo", "loopback", "docker0", "docker_gwbridge", "vethabc123", "virbr0",
		"br-124ff3ad5b65", "vnet0", "tun0", "tap0", "cni0", "flannel.1", "cali1234",
		"kube-ipvs0", "wg0", "tailscale0", "vEthernet (WSL)", "Hyper-V Virtual Ethernet",
		"VMware Network Adapter VMnet8", "Wintun", "TAP-Windows Adapter V9",
		"以太网 2 (虚拟)", "VPN Connection",
	}
	for _, name := range virtual {
		if !IsVirtualName(name) {
			t.Errorf("expected virtual: %q", name)
		}
	}

	physicalNames := []string{"eth0", "ens33", "enp0s3", "enx00e04c123456", "wlan0", "bond0", "em1"}
	for _, name := range physicalNames {
		if IsVirtualName(name) {
			t.Errorf("expected not virtual by name: %q", name)
		}
	}
}

// Operator-built bridges carry the address when member NICs are enslaved and
// left unnumbered, so only Docker's br-<12 hex> form may be rejected by name.
func TestIsVirtualNameKeepsOperatorBridges(t *testing.T) {
	for _, name := range []string{"br0", "br1", "br-lan", "br-12ab34", "br-jkw", "br-xyw"} {
		if IsVirtualName(name) {
			t.Errorf("expected reportable bridge name: %q", name)
		}
	}
	for _, name := range []string{"br-124ff3ad5b65", "br-9997c320197f"} {
		if !IsVirtualName(name) {
			t.Errorf("expected docker bridge to be virtual: %q", name)
		}
	}
}

// A container's only interface is a veth peer named eth0 with no sysfs device
// marker; rejecting it left assets with an empty private IP.
func TestHasPrimaryPrefix(t *testing.T) {
	for _, name := range []string{"eth0", "eth1", "ens33", "enp0s3", "em1", "wlan0", "bond0", "team0", "vlan100"} {
		if !hasPrimaryPrefix(name) {
			t.Errorf("expected primary prefix: %q", name)
		}
	}
	for _, name := range []string{"docker0", "veth123", "br-124ff3ad5b65", "tun0"} {
		if hasPrimaryPrefix(name) && !IsVirtualName(name) {
			t.Errorf("virtual interface slipped through: %q", name)
		}
	}
}
