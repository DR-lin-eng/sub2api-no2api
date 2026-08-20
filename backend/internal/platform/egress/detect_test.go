package egress

import (
	"errors"
	"net"
	"testing"
)

type stringAddr string

func (a stringAddr) Network() string { return "ip+net" }
func (a stringAddr) String() string  { return string(a) }

func TestDetectIPv6NetworkPrefersGlobalAddressAndDerives64(t *testing.T) {
	originalInterfaces := interfacesFunc
	originalAddrs := interfaceAddrsFunc
	t.Cleanup(func() {
		interfacesFunc = originalInterfaces
		interfaceAddrsFunc = originalAddrs
	})
	interfacesFunc = func() ([]net.Interface, error) {
		return []net.Interface{{Index: 1, Name: "eth0"}, {Index: 2, Name: "wan0"}}, nil
	}
	interfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
		if iface.Name == "eth0" {
			return []net.Addr{stringAddr("fd00::1/64"), stringAddr("fe80::1/64")}, nil
		}
		return []net.Addr{stringAddr("2001:4860:abcd:1::42/128")}, nil
	}

	got, err := DetectIPv6Network()
	if err != nil {
		t.Fatalf("DetectIPv6Network() error = %v", err)
	}
	if got.Interface != "wan0" || got.Address.String() != "2001:4860:abcd:1::42" || got.Prefix.String() != "2001:4860:abcd:1::/64" {
		t.Fatalf("unexpected detection result: %#v", got)
	}
}

func TestDetectIPv6NetworkFailsClosedWithoutGlobalAddress(t *testing.T) {
	originalInterfaces := interfacesFunc
	originalAddrs := interfaceAddrsFunc
	t.Cleanup(func() {
		interfacesFunc = originalInterfaces
		interfaceAddrsFunc = originalAddrs
	})
	interfacesFunc = func() ([]net.Interface, error) { return []net.Interface{{Name: "eth0"}}, nil }
	interfaceAddrsFunc = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{stringAddr("fd00::1/64")}, nil
	}

	if _, err := DetectIPv6Network(); !errors.Is(err, ErrIPv6AutoDetect) {
		t.Fatalf("DetectIPv6Network() error = %v, want ErrIPv6AutoDetect", err)
	}
}

func TestDetectIPv6NetworkRejectsDocumentationPrefix(t *testing.T) {
	originalInterfaces := interfacesFunc
	originalAddrs := interfaceAddrsFunc
	t.Cleanup(func() {
		interfacesFunc = originalInterfaces
		interfaceAddrsFunc = originalAddrs
	})
	interfacesFunc = func() ([]net.Interface, error) { return []net.Interface{{Name: "eth0"}}, nil }
	interfaceAddrsFunc = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{stringAddr("2001:db8::1/64")}, nil
	}
	if _, err := DetectIPv6Network(); !errors.Is(err, ErrIPv6AutoDetect) {
		t.Fatalf("DetectIPv6Network() error = %v, want ErrIPv6AutoDetect", err)
	}
}
