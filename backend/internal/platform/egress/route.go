package egress

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

type Mode string

const (
	ModeInherit       Mode = "inherit"
	ModeDirect        Mode = "direct"
	ModeExternalProxy Mode = "external_proxy"
	ModeIPv6Pool      Mode = "ipv6_pool"
)

var (
	ErrInvalidRoute       = errors.New("invalid egress route")
	ErrIPv6Disabled       = errors.New("IPv6 egress is disabled")
	ErrIPv6Unsupported    = errors.New("IPv6 egress is only supported on Linux")
	ErrIPv6Destination    = errors.New("upstream has no usable IPv6 address")
	ErrIPv6SourceRequired = errors.New("IPv6 egress source address is required")
)

// Route is the complete, account-scoped outbound network decision. It is a
// value object so callers can carry it with an already-selected account and do
// not need another database lookup on the request path.
type Route struct {
	Mode           Mode   `json:"mode"`
	ProxyURL       string `json:"proxy_url,omitempty"`
	SourceIPv6     string `json:"source_ipv6,omitempty"`
	PoolID         int64  `json:"pool_id,omitempty"`
	BindingVersion int64  `json:"binding_version,omitempty"`
	Inherited      bool   `json:"inherited,omitempty"`
}

type Policy struct {
	IPv6Enabled bool
	FreeBind    bool
}

func DirectRoute(inherited bool) Route {
	return Route{Mode: ModeDirect, Inherited: inherited}
}

func ExternalProxyRoute(proxyURL string) Route {
	return Route{Mode: ModeExternalProxy, ProxyURL: strings.TrimSpace(proxyURL)}
}

func IPv6PoolRoute(sourceIPv6 string, poolID, bindingVersion int64, inherited bool) Route {
	return Route{
		Mode:           ModeIPv6Pool,
		SourceIPv6:     strings.TrimSpace(sourceIPv6),
		PoolID:         poolID,
		BindingVersion: bindingVersion,
		Inherited:      inherited,
	}
}

func (r Route) Validate() error {
	switch r.Mode {
	case ModeDirect:
		if strings.TrimSpace(r.ProxyURL) != "" || strings.TrimSpace(r.SourceIPv6) != "" {
			return fmt.Errorf("%w: direct route contains proxy or source address", ErrInvalidRoute)
		}
		return nil
	case ModeExternalProxy:
		if strings.TrimSpace(r.ProxyURL) == "" {
			return fmt.Errorf("%w: external proxy URL is empty", ErrInvalidRoute)
		}
		return nil
	case ModeIPv6Pool:
		if strings.TrimSpace(r.SourceIPv6) == "" {
			return ErrIPv6SourceRequired
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(r.SourceIPv6))
		if err != nil || !addr.Is6() || addr.Is4In6() {
			return fmt.Errorf("%w: invalid IPv6 source address %q", ErrInvalidRoute, r.SourceIPv6)
		}
		if r.PoolID <= 0 || r.BindingVersion <= 0 {
			return fmt.Errorf("%w: IPv6 route has invalid pool or binding version", ErrInvalidRoute)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported mode %q", ErrInvalidRoute, r.Mode)
	}
}

// ApplyPolicy implements the runtime kill switch. Inherited IPv6 routes become
// direct when the feature is disabled, while explicitly selected IPv6 routes
// fail closed so an administrator cannot unknowingly leak them over IPv4.
func ApplyPolicy(route Route, policy Policy) (Route, error) {
	if route.Mode == ModeIPv6Pool && !policy.IPv6Enabled {
		if route.Inherited {
			return DirectRoute(true), nil
		}
		return Route{}, ErrIPv6Disabled
	}
	if err := route.Validate(); err != nil {
		return Route{}, err
	}
	return route, nil
}

func (r Route) CacheKey() string {
	return fmt.Sprintf("%s|%s|%s|%d|%d", r.Mode, strings.TrimSpace(r.ProxyURL), strings.TrimSpace(r.SourceIPv6), r.PoolID, r.BindingVersion)
}
