package netiface

import "testing"

func TestIsVirtualName(t *testing.T) {
	virtual := []string{
		"lo", "loopback", "docker0", "docker_gwbridge", "vethabc123", "virbr0",
		"br-12ab34", "vnet0", "tun0", "tap0", "cni0", "flannel.1", "cali1234",
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
