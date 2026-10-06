package urlvalidator

import (
	"net/netip"
	"testing"
)

func TestValidatePublicIP(t *testing.T) {
	for _, raw := range []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "100.100.100.200", "127.0.0.1",
		"169.254.169.254", "172.16.0.1", "192.168.1.1", "192.0.0.1",
		"192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "fc00::1",
		"fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:169.254.169.254",
		"64:ff9b::a9fe:a9fe", "64:ff9b:1::a9fe:a9fe", "2002:a9fe:a9fe::1",
		"2001::1", "2001:db8::1", "100::1", "fe80::1%eth0",
	} {
		t.Run(raw, func(t *testing.T) {
			if err := ValidatePublicIP(netip.MustParseAddr(raw)); err == nil {
				t.Fatal("unsafe IP was accepted")
			}
		})
	}
	for _, raw := range []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if err := ValidatePublicIP(netip.MustParseAddr(raw)); err != nil {
			t.Fatalf("public IP %s rejected: %v", raw, err)
		}
	}
	if err := ValidatePublicIP(netip.Addr{}); err == nil {
		t.Fatal("invalid IP was accepted")
	}
}

func TestValidatePublicHTTPURL(t *testing.T) {
	for _, raw := range []string{
		"", "/image.png", "file:///etc/passwd", "gopher://example.com/",
		"https://user:password@example.com/image.png", "https://localhost/",
		"https://LOCALHOST./", "https://a.localhost/", "https://127.0.0.1/",
		"https://10.0.0.1/", "http://169.254.169.254/latest/meta-data/",
		"https://100.100.100.200/", "https://[::ffff:127.0.0.1]/",
		"https://[fe80::1%25eth0]/", "https://example.com:0/", "https://example.com:65536/",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ValidatePublicHTTPURL(raw, true); err == nil {
				t.Fatal("unsafe URL was accepted")
			}
		})
	}
	for _, raw := range []string{"https://cdn.example.com/a%2Fb.png?sig=123", "http://cdn.example.com/image.png/"} {
		normalized, err := ValidatePublicHTTPURL(raw, true)
		if err != nil || normalized != raw {
			t.Fatalf("public URL changed or rejected: %q %v", normalized, err)
		}
	}
	if _, err := ValidatePublicHTTPURL("http://example.com/", false); err == nil {
		t.Fatal("HTTP policy was ignored")
	}
}
