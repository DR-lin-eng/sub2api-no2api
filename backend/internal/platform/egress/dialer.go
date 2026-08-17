package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultDialTimeout   = 10 * time.Second
	DefaultDialKeepAlive = 30 * time.Second
)

type IPResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type DialerOptions struct {
	Timeout   time.Duration
	KeepAlive time.Duration
	Resolver  IPResolver
}

func NewDialContext(route Route, policy Policy, opts DialerOptions) (func(context.Context, string, string) (net.Conn, error), error) {
	effective, err := ApplyPolicy(route, policy)
	if err != nil {
		return nil, err
	}
	if effective.Mode == ModeExternalProxy {
		return nil, fmt.Errorf("%w: proxy routes must be configured by the proxy transport", ErrInvalidRoute)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	keepAlive := opts.KeepAlive
	if keepAlive <= 0 {
		keepAlive = DefaultDialKeepAlive
	}
	base := &net.Dialer{Timeout: timeout, KeepAlive: keepAlive}
	if effective.Mode == ModeDirect {
		return base.DialContext, nil
	}
	if runtime.GOOS != "linux" {
		return nil, ErrIPv6Unsupported
	}

	source, err := netip.ParseAddr(effective.SourceIPv6)
	if err != nil || !source.Is6() || source.Is4In6() {
		return nil, fmt.Errorf("%w: %q", ErrIPv6SourceRequired, effective.SourceIPv6)
	}
	base.LocalAddr = &net.TCPAddr{IP: net.IP(source.AsSlice())}
	if policy.FreeBind {
		base.Control = func(_, _ string, raw syscall.RawConn) error {
			return enableIPv6FreeBind(raw)
		}
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return func(ctx context.Context, _ string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split upstream address: %w", err)
		}
		addresses, err := resolveIPv6(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		var dialErrors []error
		for _, addr := range addresses {
			conn, dialErr := base.DialContext(ctx, "tcp6", net.JoinHostPort(addr.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		return nil, fmt.Errorf("%w for %s: %v", ErrIPv6Destination, host, errors.Join(dialErrors...))
	}, nil
}

func resolveIPv6(ctx context.Context, resolver IPResolver, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrIPv6Destination)
	}
	if literal, err := netip.ParseAddr(host); err == nil {
		if literal.Is6() && !literal.Is4In6() {
			return []netip.Addr{literal}, nil
		}
		return nil, fmt.Errorf("%w: %s is not IPv6", ErrIPv6Destination, host)
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip6", host)
	if err != nil {
		return nil, fmt.Errorf("%w for %s: %v", ErrIPv6Destination, host, err)
	}
	out := make([]netip.Addr, 0, len(addresses))
	for _, addr := range addresses {
		if addr.Is6() && !addr.Is4In6() {
			out = append(out, addr.Unmap())
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w for %s", ErrIPv6Destination, host)
	}
	return out, nil
}
