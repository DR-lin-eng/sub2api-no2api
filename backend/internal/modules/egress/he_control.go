package egress

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

const (
	HETunnelActionApply  = "apply"
	HETunnelActionCheck  = "check"
	HETunnelActionRemove = "remove"
)

var ErrHETunnelControlUnavailable = errors.New("HE tunnel control is unavailable")

type HETunnelConfig struct {
	Enabled             bool   `json:"enabled"`
	ServerIPv4          string `json:"server_ipv4"`
	LocalIPv4           string `json:"local_ipv4,omitempty"`
	ClientIPv6          string `json:"client_ipv6"`
	ServerIPv6          string `json:"server_ipv6"`
	PoolCIDR            string `json:"pool_cidr"`
	MTU                 int    `json:"mtu"`
	RouteMetric         int    `json:"route_metric"`
	ProbeIPv6           string `json:"probe_ipv6,omitempty"`
	ProbeTimeoutSeconds int    `json:"probe_timeout_seconds"`
	AllowPrivateIPv4    bool   `json:"allow_private_ipv4"`
	UpdateEnabled       bool   `json:"update_enabled"`
	TunnelID            string `json:"tunnel_id,omitempty"`
	Username            string `json:"username,omitempty"`
	UpdateKeyConfigured bool   `json:"update_key_configured"`
	UpdateKey           string `json:"-"`
}

type SaveHETunnelConfigInput struct {
	Enabled             bool   `json:"enabled"`
	ServerIPv4          string `json:"server_ipv4"`
	LocalIPv4           string `json:"local_ipv4"`
	ClientIPv6          string `json:"client_ipv6"`
	ServerIPv6          string `json:"server_ipv6"`
	PoolCIDR            string `json:"pool_cidr"`
	MTU                 int    `json:"mtu"`
	RouteMetric         int    `json:"route_metric"`
	ProbeIPv6           string `json:"probe_ipv6"`
	ProbeTimeoutSeconds int    `json:"probe_timeout_seconds"`
	AllowPrivateIPv4    bool   `json:"allow_private_ipv4"`
	UpdateEnabled       bool   `json:"update_enabled"`
	TunnelID            string `json:"tunnel_id"`
	Username            string `json:"username"`
	UpdateKey           string `json:"update_key"`
	ClearUpdateKey      bool   `json:"clear_update_key"`
}

type HETunnelAgentStatus struct {
	Online    bool       `json:"online"`
	State     string     `json:"state"`
	Action    string     `json:"action,omitempty"`
	Message   string     `json:"message,omitempty"`
	RequestID string     `json:"request_id,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type HETunnelControlSnapshot struct {
	Available bool                `json:"available"`
	Config    HETunnelConfig      `json:"config"`
	Agent     HETunnelAgentStatus `json:"agent"`
}

type HETunnelControlStore interface {
	Load(ctx context.Context) (*HETunnelControlSnapshot, error)
	SaveConfig(ctx context.Context, config HETunnelConfig) error
	Request(ctx context.Context, action string) (string, error)
}

type HETunnelControlService struct {
	store HETunnelControlStore
	cfg   *config.Config
}

func NewHETunnelControlService(store HETunnelControlStore, cfg *config.Config) *HETunnelControlService {
	return &HETunnelControlService{store: store, cfg: cfg}
}

func (s *HETunnelControlService) Get(ctx context.Context) (*HETunnelControlSnapshot, error) {
	if !s.available() {
		return &HETunnelControlSnapshot{
			Available: false,
			Config:    defaultHETunnelConfig(),
			Agent:     HETunnelAgentStatus{State: "unavailable"},
		}, nil
	}
	snapshot, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	snapshot.Available = true
	normalizeHETunnelDefaults(&snapshot.Config)
	snapshot.Config.UpdateKeyConfigured = strings.TrimSpace(snapshot.Config.UpdateKey) != ""
	snapshot.Config.UpdateKey = ""
	return snapshot, nil
}

func (s *HETunnelControlService) Save(ctx context.Context, input SaveHETunnelConfigInput) (*HETunnelControlSnapshot, error) {
	if !s.available() {
		return nil, ErrHETunnelControlUnavailable
	}
	current, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	next := HETunnelConfig{
		Enabled:             input.Enabled,
		ServerIPv4:          strings.TrimSpace(input.ServerIPv4),
		LocalIPv4:           strings.TrimSpace(input.LocalIPv4),
		ClientIPv6:          strings.TrimSpace(input.ClientIPv6),
		ServerIPv6:          strings.TrimSpace(input.ServerIPv6),
		PoolCIDR:            strings.TrimSpace(input.PoolCIDR),
		MTU:                 input.MTU,
		RouteMetric:         input.RouteMetric,
		ProbeIPv6:           strings.TrimSpace(input.ProbeIPv6),
		ProbeTimeoutSeconds: input.ProbeTimeoutSeconds,
		AllowPrivateIPv4:    input.AllowPrivateIPv4,
		UpdateEnabled:       input.UpdateEnabled,
		TunnelID:            strings.TrimSpace(input.TunnelID),
		Username:            strings.TrimSpace(input.Username),
	}
	normalizeHETunnelDefaults(&next)
	if !input.ClearUpdateKey {
		next.UpdateKey = strings.TrimSpace(input.UpdateKey)
		if next.UpdateKey == "" {
			next.UpdateKey = current.Config.UpdateKey
		}
	}
	if err := validateHETunnelConfig(next, next.Enabled); err != nil {
		return nil, err
	}
	if err := s.store.SaveConfig(ctx, next); err != nil {
		return nil, err
	}
	return s.Get(ctx)
}

func (s *HETunnelControlService) Request(ctx context.Context, action string) (*HETunnelControlSnapshot, error) {
	if !s.available() {
		return nil, ErrHETunnelControlUnavailable
	}
	action = strings.TrimSpace(action)
	switch action {
	case HETunnelActionApply, HETunnelActionCheck:
		if !s.runtimeEnabled() {
			return nil, ErrRuntimeUnavailable
		}
		current, err := s.store.Load(ctx)
		if err != nil {
			return nil, err
		}
		if !current.Config.Enabled {
			return nil, fmt.Errorf("HE tunnel configuration must be enabled before %s", action)
		}
		if err := validateHETunnelConfig(current.Config, true); err != nil {
			return nil, err
		}
	case HETunnelActionRemove:
	default:
		return nil, fmt.Errorf("invalid HE tunnel action %q", action)
	}
	if _, err := s.store.Request(ctx, action); err != nil {
		return nil, err
	}
	return s.Get(ctx)
}

func (s *HETunnelControlService) available() bool {
	return s != nil && s.store != nil && s.cfg != nil && s.cfg.IPv6Egress.ControlEnabled
}

func (s *HETunnelControlService) runtimeEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.IPv6Egress.IsEnabled()
}

// DisableRuntime makes the sidecar converge to an absent tunnel when the
// administrator turns the IPv6 master switch off. Removal is best effort: the
// persisted switch remains authoritative and the sidecar can retry on its next
// heartbeat if the control volume is temporarily unavailable.
func (s *HETunnelControlService) DisableRuntime(ctx context.Context) {
	if !s.available() {
		return
	}
	current, err := s.store.Load(ctx)
	if err != nil || current == nil {
		return
	}
	if current.Config.Enabled {
		current.Config.Enabled = false
		if err := s.store.SaveConfig(ctx, current.Config); err != nil {
			return
		}
	}
	_, _ = s.store.Request(ctx, HETunnelActionRemove)
}

func defaultHETunnelConfig() HETunnelConfig {
	return HETunnelConfig{
		MTU:                 1480,
		RouteMetric:         2048,
		ProbeIPv6:           "2606:4700:4700::1111",
		ProbeTimeoutSeconds: 5,
		AllowPrivateIPv4:    true,
	}
}

func normalizeHETunnelDefaults(value *HETunnelConfig) {
	if value == nil {
		return
	}
	if value.MTU == 0 {
		value.MTU = 1480
	}
	if value.RouteMetric == 0 {
		value.RouteMetric = 2048
	}
	if value.ProbeTimeoutSeconds == 0 {
		value.ProbeTimeoutSeconds = 5
	}
}

func validateHETunnelConfig(value HETunnelConfig, requireComplete bool) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "server_ipv4", value: value.ServerIPv4},
		{name: "local_ipv4", value: value.LocalIPv4},
		{name: "client_ipv6", value: value.ClientIPv6},
		{name: "server_ipv6", value: value.ServerIPv6},
		{name: "pool_cidr", value: value.PoolCIDR},
		{name: "probe_ipv6", value: value.ProbeIPv6},
		{name: "tunnel_id", value: value.TunnelID},
		{name: "username", value: value.Username},
		{name: "update_key", value: value.UpdateKey},
	}
	for _, field := range fields {
		name, raw := field.name, field.value
		if strings.ContainsAny(raw, "\r\n\x00") {
			return fmt.Errorf("HE tunnel %s contains unsupported control characters", name)
		}
	}
	if requireComplete {
		for _, field := range fields {
			if field.name == "local_ipv4" || field.name == "probe_ipv6" || field.name == "tunnel_id" || field.name == "username" || field.name == "update_key" {
				continue
			}
			if field.value == "" {
				return fmt.Errorf("HE tunnel %s is required", field.name)
			}
		}
	}

	serverIPv4, err := parseOptionalAddr(value.ServerIPv4, true)
	if err != nil {
		return fmt.Errorf("invalid HE tunnel server_ipv4: %w", err)
	}
	if serverIPv4.IsValid() && isNonPublicIPv4(serverIPv4) {
		return fmt.Errorf("HE tunnel server_ipv4 must be publicly routable")
	}
	localIPv4, err := parseOptionalAddr(value.LocalIPv4, true)
	if err != nil {
		return fmt.Errorf("invalid HE tunnel local_ipv4: %w", err)
	}
	if localIPv4.IsValid() && isNonPublicIPv4(localIPv4) && !value.AllowPrivateIPv4 {
		return fmt.Errorf("HE tunnel local_ipv4 is private; enable allow_private_ipv4 only for container NAT or verified protocol 41 forwarding")
	}
	serverIPv6, err := parseOptionalAddr(value.ServerIPv6, false)
	if err != nil {
		return fmt.Errorf("invalid HE tunnel server_ipv6: %w", err)
	}
	if serverIPv6.IsValid() && (!serverIPv6.IsGlobalUnicast() || serverIPv6.IsPrivate()) {
		return fmt.Errorf("HE tunnel server_ipv6 must be globally routable")
	}
	if _, err := parseOptionalAddr(value.ProbeIPv6, false); err != nil {
		return fmt.Errorf("invalid HE tunnel probe_ipv6: %w", err)
	}

	var clientPrefix netip.Prefix
	if value.ClientIPv6 != "" {
		clientPrefix, err = netip.ParsePrefix(value.ClientIPv6)
		if err != nil || !clientPrefix.Addr().Is6() || clientPrefix.Addr().Is4In6() || clientPrefix.Bits() != 64 {
			return fmt.Errorf("HE tunnel client_ipv6 must be an IPv6 /64 prefix address")
		}
		clientPrefix = clientPrefix.Masked()
	}
	var poolPrefix netip.Prefix
	if value.PoolCIDR != "" {
		poolPrefix, err = ValidatePoolCIDR(value.PoolCIDR)
		if err != nil {
			return err
		}
		if poolPrefix.Bits() < 48 {
			return fmt.Errorf("HE routed pool must be /48 or smaller in address count")
		}
	}
	if clientPrefix.IsValid() && poolPrefix.IsValid() && clientPrefix.Overlaps(poolPrefix) {
		return fmt.Errorf("HE routed pool must be different from the tunnel /64")
	}
	if value.MTU < 1280 || value.MTU > 1480 {
		return fmt.Errorf("HE tunnel mtu must be between 1280 and 1480")
	}
	if value.RouteMetric < 1 || value.RouteMetric > 65535 {
		return fmt.Errorf("HE tunnel route_metric must be between 1 and 65535")
	}
	if value.ProbeTimeoutSeconds < 1 || value.ProbeTimeoutSeconds > 30 {
		return fmt.Errorf("HE tunnel probe_timeout_seconds must be between 1 and 30")
	}
	if len(value.Username) > 256 || len(value.UpdateKey) > 512 {
		return fmt.Errorf("HE tunnel update credentials are too long")
	}
	if value.UpdateEnabled && requireComplete {
		id, err := strconv.ParseUint(value.TunnelID, 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("HE tunnel tunnel_id must be a positive integer when endpoint updates are enabled")
		}
		if value.Username == "" || value.UpdateKey == "" {
			return fmt.Errorf("HE tunnel username and update_key are required when endpoint updates are enabled")
		}
	}
	return nil
}

func parseOptionalAddr(raw string, wantIPv4 bool) (netip.Addr, error) {
	if raw == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, err
	}
	if wantIPv4 != addr.Is4() || addr.Is4In6() {
		if wantIPv4 {
			return netip.Addr{}, fmt.Errorf("must be IPv4")
		}
		return netip.Addr{}, fmt.Errorf("must be IPv6")
	}
	return addr, nil
}

func isNonPublicIPv4(addr netip.Addr) bool {
	if !addr.Is4() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsMulticast() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(addr)
}
