// Package netiface identifies host network interfaces that are suitable for
// inventory and monitoring. Keeping this policy here makes netinfo and metrics
// use exactly the same interface set.
package netiface

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Virtual / container / hypervisor interface name prefixes (lowercase).
var virtualPrefixes = []string{
	"docker", "veth", "virbr", "vnet", "br-", "kube-", "flannel", "cni", "cali",
	"tun", "tap", "dummy", "sit", "ip6tnl", "gre", "gretap", "erspan", "ifb",
	"vboxnet", "vmnet", "podman", "lxc", "weave", "cilium", "nodelocaldns",
	"wg", "tailscale", "zt", "nebula",
}

// Substrings common on Windows virtual adapters (lowercase).
var virtualContains = []string{
	"vethernet", "hyper-v", "virtualbox", "vmware", "wsl", "teredo", "isatap",
	"bluetooth", "npcap loopback", "loopback", "pseudo-interface", "wan miniport",
	"docker", "wintun", "tap-windows", "tap-win", "openvpn", "softether",
	"host-only", "host only", "virtual", "vpn", "wireguard", "zerotier",
	"virtual adapter", "虚拟",
}

// IsReportable reports whether an interface should be used for IP inventory
// and traffic monitoring.
//
// On Linux, known virtual names are rejected first, then sysfs must identify
// the interface as a bus-backed device, Wi-Fi interface, bond, or team.
// On other systems, loopback and known virtual adapter names are rejected.
func IsReportable(name string) bool {
	if name == "" || IsVirtualName(name) {
		return false
	}
	if runtime.GOOS == "linux" {
		return linuxIsReal(name)
	}
	return true
}

// IsVirtualName reports whether an interface name is known to be virtual.
func IsVirtualName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" || lower == "lo" || strings.HasPrefix(lower, "lo:") || strings.HasPrefix(lower, "loop") {
		return true
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, fragment := range virtualContains {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func linuxIsReal(name string) bool {
	base := filepath.Join("/sys/class/net", name)
	for _, marker := range []string{"device", "wireless", "bonding", "team"} {
		if _, err := os.Stat(filepath.Join(base, marker)); err == nil {
			return true
		}
	}
	return false
}
