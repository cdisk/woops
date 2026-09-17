// Package netiface identifies host network interfaces that are suitable for
// inventory and monitoring. Keeping this policy here makes netinfo and metrics
// use exactly the same interface set.
package netiface

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Virtual / container / hypervisor interface name prefixes (lowercase).
var virtualPrefixes = []string{
	"docker", "veth", "virbr", "vnet", "kube-", "flannel", "cni", "cali",
	"tun", "tap", "dummy", "sit", "ip6tnl", "gre", "gretap", "erspan", "ifb",
	"vboxnet", "vmnet", "podman", "lxc", "weave", "cilium", "nodelocaldns",
	"wg", "tailscale", "zt", "nebula",
}

// Docker names each user-defined network's bridge br-<first 12 of network id>.
// Matched exactly rather than by a "br-" prefix, so operator-built bridges
// (br0, br-lan) keep reporting their addresses.
var dockerBridgeRe = regexp.MustCompile(`^br-[0-9a-f]{12}$`)

// Conventional primary-NIC name prefixes, used only as a fallback when sysfs
// gives no verdict - notably inside a container, where the sole interface is a
// veth peer named eth0 and has no /sys/class/net/eth0/device.
var primaryPrefixes = []string{"eth", "en", "em", "wl", "ib", "bond", "team", "vlan"}

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
// the interface as a bus-backed device, Wi-Fi interface, bond, team, operator
// bridge, or stacked interface; failing that, a conventional primary-NIC name
// is accepted so containers still report their own address.
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
	if dockerBridgeRe.MatchString(lower) {
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
	// Bus-backed NIC, Wi-Fi, bond or team. "bridge" covers an operator-built
	// bridge (br0/br1), which is where the address lives when the member NICs
	// are enslaved and left unnumbered; Docker's own bridges are already
	// rejected by name.
	for _, marker := range []string{"device", "wireless", "bonding", "team", "bridge"} {
		if _, err := os.Stat(filepath.Join(base, marker)); err == nil {
			return true
		}
	}
	// Stacked interface such as a VLAN: no bus of its own, but it has a parent
	// and usually carries the address instead of that parent.
	if lowers, _ := filepath.Glob(filepath.Join(base, "lower_*")); len(lowers) > 0 {
		return true
	}
	// No sysfs verdict. Inside a container the only interface is a veth peer
	// named eth0, so refusing here would leave the asset with no address at all.
	return hasPrimaryPrefix(name)
}

func hasPrimaryPrefix(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range primaryPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
