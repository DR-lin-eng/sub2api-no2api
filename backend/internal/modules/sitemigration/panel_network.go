package sitemigration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/urlvalidator"
)

var ErrPeerAddress = errors.New("migration peer address is not permitted")

type peerResponseError struct {
	action string
	status int
}

func (e *peerResponseError) Error() string {
	return fmt.Sprintf("peer rejected %s (HTTP %d)", e.action, e.status)
}

func canonicalPeer(raw string) (string, error) {
	if len(raw) > 512 {
		return "", ErrPeerAddress
	}
	origin, err := peerOrigin(strings.TrimSpace(raw))
	if err != nil {
		return "", ErrPeerAddress
	}
	u, err := url.Parse(origin)
	if err != nil {
		return "", ErrPeerAddress
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	port := u.Port()
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.ContainsAny(host, "% /\\") {
		return "", ErrPeerAddress
	}
	if port != "" && port != "443" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	return u.String(), nil
}
func (p *Panel) allowedIP(origin string, ip netip.Addr) bool {
	ip = ip.Unmap()
	return urlvalidator.ValidatePublicIP(ip) == nil || (p.privatePeers[origin] && ip.Zone() == "" && ip.IsPrivate())
}
func (p *Panel) peerOrigin(raw string) (string, error) {
	o, err := canonicalPeer(raw)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(o)
	if err != nil {
		return "", ErrPeerAddress
	}
	if ip, e := netip.ParseAddr(u.Hostname()); e == nil && !p.allowedIP(o, ip) {
		return "", ErrPeerAddress
	}
	return o, nil
}

// Validate every DNS answer at connection time and dial the IP directly.
// net/http retains the original hostname for certificate verification.
func (p *Panel) dialPeer(origin string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, ErrPeerAddress
		}
		originURL, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(originURL.Hostname(), host) {
			return nil, ErrPeerAddress
		}
		resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(resolved) == 0 {
			return nil, ErrPeerAddress
		}
		for _, ip := range resolved {
			if !p.allowedIP(origin, ip) {
				return nil, ErrPeerAddress
			}
		}
		d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		for _, ip := range resolved {
			var c net.Conn
			c, err = d.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return c, nil
			}
		}
		return nil, err
	}
}
