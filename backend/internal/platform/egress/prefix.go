package egress

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

// PrefixCandidate is an IPv6 network observed in the current network
// namespace. Tunnel interfaces are reported for diagnostics but are never the
// recommended account pool because a HE point-to-point /64 is not a routed
// address pool.
type PrefixCandidate struct {
	Prefix    string `json:"prefix"`
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Global    bool   `json:"global"`
	Tunnel    bool   `json:"tunnel"`
	Usable    bool   `json:"usable"`
	Reason    string `json:"reason,omitempty"`
}

// DerivePoolPrefix normalizes an observed address/prefix into a pool-sized
// IPv6 prefix. A bare address has no routing metadata, so the caller may opt
// into the conventional /64 default; an explicitly supplied broad prefix is
// narrowed to a containing /64, while host prefixes are never broadened.
func DerivePoolPrefix(raw string, preferredBits int) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("IPv6 address or prefix is required")
	}
	prefix, err := netip.ParsePrefix(raw)
	bareAddress := false
	if err != nil {
		addr, addrErr := netip.ParseAddr(raw)
		if addrErr != nil || !addr.Is6() || addr.Is4In6() {
			return netip.Prefix{}, fmt.Errorf("invalid IPv6 address or prefix %q", raw)
		}
		prefix = netip.PrefixFrom(addr, 128)
		bareAddress = true
	}
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("invalid IPv6 address or prefix %q", raw)
	}
	bits := prefix.Bits()
	originalBits := bits
	if preferredBits == 0 {
		preferredBits = 64
	}
	if preferredBits < 48 || preferredBits > 120 {
		return netip.Prefix{}, fmt.Errorf("pool prefix length must be between 48 and 120")
	}
	// A bare address is commonly returned without route metadata by cloud APIs.
	// Broad provider prefixes are narrowed to a containing pool-sized prefix so
	// the suggestion does not claim the provider's entire allocation.
	if bareAddress || bits < preferredBits {
		bits = preferredBits
	}
	if originalBits < 48 {
		return netip.Prefix{}, fmt.Errorf("observed IPv6 prefix %s is broader than /48", prefix)
	}
	if bits > 120 {
		return netip.Prefix{}, fmt.Errorf("observed IPv6 prefix %s is too small for a pool", prefix)
	}
	return netip.PrefixFrom(prefix.Addr(), bits).Masked(), nil
}

// DiscoverPrefixes inspects interface addresses in the current network
// namespace. It intentionally does not perform an external request: the
// address must be locally routed before it can be used as an account source.
func DiscoverPrefixes() ([]PrefixCandidate, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list IPv6 interfaces: %w", err)
	}
	byKey := make(map[string]PrefixCandidate)
	for _, iface := range interfaces {
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		tunnel := isTunnelInterface(iface.Name)
		for _, rawAddr := range addrs {
			prefix, addr, ok := parseInterfaceIPv6(rawAddr.String())
			if !ok || !addr.Is6() || addr.Is4In6() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() {
				continue
			}
			candidatePrefix, deriveErr := DerivePoolPrefix(prefix.String(), 64)
			if deriveErr != nil {
				continue
			}
			global := addr.IsGlobalUnicast() && !addr.IsPrivate()
			candidate := PrefixCandidate{
				Prefix:    candidatePrefix.String(),
				Interface: iface.Name,
				Address:   addr.String(),
				Global:    global,
				Tunnel:    tunnel,
				Usable:    global && !tunnel,
				Reason:    prefixReason(global, tunnel),
			}
			key := candidate.Interface + "|" + candidate.Prefix
			if prior, exists := byKey[key]; !exists || (!prior.Global && candidate.Global) {
				byKey[key] = candidate
			}
		}
	}
	result := make([]PrefixCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Usable != result[j].Usable {
			return result[i].Usable
		}
		if result[i].Global != result[j].Global {
			return result[i].Global
		}
		if result[i].Interface != result[j].Interface {
			return result[i].Interface < result[j].Interface
		}
		return result[i].Prefix < result[j].Prefix
	})
	return result, nil
}

// DiscoverPoolPrefix returns the best locally observed non-tunnel prefix.
func DiscoverPoolPrefix() (netip.Prefix, PrefixCandidate, error) {
	candidates, err := DiscoverPrefixes()
	if err != nil {
		return netip.Prefix{}, PrefixCandidate{}, err
	}
	for _, candidate := range candidates {
		if candidate.Usable {
			prefix, parseErr := netip.ParsePrefix(candidate.Prefix)
			if parseErr == nil {
				return prefix, candidate, nil
			}
		}
	}
	return netip.Prefix{}, PrefixCandidate{}, fmt.Errorf("no locally routed global IPv6 prefix was found")
}

func parseInterfaceIPv6(raw string) (netip.Prefix, netip.Addr, bool) {
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(raw)); err == nil {
		return prefix, prefix.Addr(), true
	}
	if ip, network, err := net.ParseCIDR(strings.TrimSpace(raw)); err == nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return netip.Prefix{}, netip.Addr{}, false
		}
		bits, _ := network.Mask.Size()
		return netip.PrefixFrom(addr, bits), addr, true
	}
	return netip.Prefix{}, netip.Addr{}, false
}

func isTunnelInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "he-") || strings.HasPrefix(name, "sit") || strings.HasPrefix(name, "ip6tnl") || strings.Contains(name, "6in4")
}

func prefixReason(global, tunnel bool) string {
	if tunnel {
		return "point-to-point tunnel prefix; configure a separately routed HE pool"
	}
	if !global {
		return "private or ULA prefix; use only for local/Docker validation"
	}
	return "locally routed global prefix"
}
