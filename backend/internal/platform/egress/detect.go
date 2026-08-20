package egress

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

var ErrIPv6AutoDetect = errors.New("no globally routable IPv6 network was detected")

// DetectedIPv6Network is the smallest useful description of the IPv6 network
// visible from the application network namespace. Prefix is deliberately
// conservative: a /64 is derived from the address instead of guessing a
// provider-wide /48 or /56 that may not be routed to this host.
type DetectedIPv6Network struct {
	Address   netip.Addr   `json:"address"`
	Prefix    netip.Prefix `json:"prefix"`
	Interface string       `json:"interface"`
}

var interfacesFunc = net.Interfaces
var interfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }

// DetectIPv6Network inspects interfaces in the current network namespace. It
// excludes loopback, link-local, ULA and documentation addresses because none
// can be used as an Internet IPv6 egress pool.
func DetectIPv6Network() (*DetectedIPv6Network, error) {
	interfaces, err := interfacesFunc()
	if err != nil {
		return nil, fmt.Errorf("list IPv6 interfaces: %w", err)
	}
	var candidates []DetectedIPv6Network
	for _, iface := range interfaces {
		addresses, err := interfaceAddrsFunc(iface)
		if err != nil {
			continue
		}
		for _, raw := range addresses {
			ip, _, ok := parseInterfaceIPv6(raw)
			if !ok || !isUsableGlobalIPv6(ip) {
				continue
			}
			// A delegated prefix is normally presented as /64 on the host. If
			// the interface reports /128, derive its /64; this is still bounded
			// and lets the later source probe reject unrouted networks.
			bits := 64
			candidate := DetectedIPv6Network{
				Address:   ip,
				Prefix:    netip.PrefixFrom(ip, bits).Masked(),
				Interface: iface.Name,
			}
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrIPv6AutoDetect
	}
	// Use a stable interface/address order so repeated auto-configure calls are
	// idempotent when a host has multiple public IPv6 addresses.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Interface != candidates[j].Interface {
			return candidates[i].Interface < candidates[j].Interface
		}
		return candidates[i].Address.String() < candidates[j].Address.String()
	})
	return &candidates[0], nil
}

func parseInterfaceIPv6(raw net.Addr) (netip.Addr, netip.Prefix, bool) {
	switch value := raw.(type) {
	case *net.IPNet:
		ip, ok := netip.AddrFromSlice(value.IP)
		if !ok || !ip.Is6() || ip.Is4In6() {
			return netip.Addr{}, netip.Prefix{}, false
		}
		bits, _ := value.Mask.Size()
		if bits <= 0 {
			bits = 128
		}
		return ip.Unmap(), netip.PrefixFrom(ip.Unmap(), bits), true
	case *net.IPAddr:
		ip, ok := netip.AddrFromSlice(value.IP)
		if !ok || !ip.Is6() || ip.Is4In6() {
			return netip.Addr{}, netip.Prefix{}, false
		}
		return ip.Unmap(), netip.PrefixFrom(ip.Unmap(), 128), true
	default:
		text := strings.TrimSpace(raw.String())
		prefix, err := netip.ParsePrefix(text)
		if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
			return netip.Addr{}, netip.Prefix{}, false
		}
		return prefix.Addr().Unmap(), prefix, true
	}
}

func isUsableGlobalIPv6(ip netip.Addr) bool {
	documentation := netip.MustParsePrefix("2001:db8::/32")
	return ip.IsValid() && ip.Is6() && !ip.Is4In6() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() && !documentation.Contains(ip)
}
