package core

import (
	"net"
	"sort"
	"strings"

	"github.com/ops-bastion/ops/go/internal/hostinfo/netiface"
)

type NetInfo struct {
	PrivateIPs []string
}

// CollectNetInfo enumerates host IPv4 addresses for netinfo reporting.
// Public/source IP is assigned by Gateway from the control WSS TCP peer - Agents
// must not probe third-party "what is my IP" services.
//
// Addresses come from reportable (non-virtual) interfaces: RFC1918 first, then
// other global unicast (enterprise intranets often use non-RFC1918 space such as
// 188.x). Link-local and loopback are excluded.
func CollectNetInfo() NetInfo {
	return NetInfo{PrivateIPs: collectPrivateIPv4()}
}

func (n NetInfo) PrivateIPCSV() string {
	return strings.Join(n.PrivateIPs, ",")
}

func collectPrivateIPv4() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var rfc1918, other []string
	seen := map[string]struct{}{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 ||
			!netiface.IsReportable(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || !ip.IsGlobalUnicast() {
				continue
			}
			s := ip.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			if ip.IsPrivate() {
				rfc1918 = append(rfc1918, s)
			} else {
				other = append(other, s)
			}
		}
	}
	sort.Strings(rfc1918)
	sort.Strings(other)
	return append(rfc1918, other...)
}
