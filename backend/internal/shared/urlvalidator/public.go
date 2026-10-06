package urlvalidator

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Public image inputs have a separate trust boundary from administrator-owned
// upstreams. Never let allow_private_hosts relax this policy.
var nonPublicIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// ValidatePublicHTTPURL checks syntax and literal destinations without resolving
// DNS. The transport must validate DNS answers and connect to those same IPs.
func ValidatePublicHTTPURL(raw string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("invalid public image URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && (!allowHTTP || parsed.Scheme != "http") {
		return "", fmt.Errorf("invalid public image URL scheme")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("public image URL must not contain credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.Contains(host, "%") {
		return "", fmt.Errorf("public image URL host is not allowed")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if err := ValidatePublicIP(ip); err != nil {
			return "", err
		}
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid public image URL port")
		}
	}
	return parsed.String(), nil
}

// ValidatePublicIP rejects local, metadata, shared and special-use destinations,
// including IPv4-mapped IPv6 and translation/tunnel ranges.
func ValidatePublicIP(ip netip.Addr) error {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("public image destination IP is not allowed")
	}
	for _, prefix := range nonPublicIPPrefixes {
		if prefix.Contains(ip) {
			return fmt.Errorf("public image destination IP is not allowed")
		}
	}
	return nil
}
