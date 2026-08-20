package egress

import (
	"context"
	"errors"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

// RuntimeSettingKey is intentionally owned by this module so the runtime
// switch does not depend on the application settings package. The value is a
// regular persisted setting and is never exposed as a secret.
const RuntimeSettingKey = "ipv6_egress_enabled"

const allocationSecretSettingKey = "ipv6_egress_allocation_secret"

type RuntimeSettings interface {
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

const (
	PoolStatusActive   = "active"
	PoolStatusDisabled = "disabled"

	BindingStatusActive   = "active"
	BindingStatusDisabled = "disabled"
)

var (
	ErrPoolNotFound       = errors.New("IPv6 egress pool not found")
	ErrBindingNotFound    = errors.New("account IPv6 egress binding not found")
	ErrAddressConflict    = errors.New("IPv6 egress address already allocated")
	ErrPoolOverlap        = errors.New("IPv6 egress pool overlaps an existing pool")
	ErrPoolUnhealthy      = errors.New("IPv6 egress pool has not passed an exit probe")
	ErrBindingChanged     = errors.New("account IPv6 egress binding changed concurrently")
	ErrPoolDisabled       = errors.New("IPv6 egress pool is disabled")
	ErrAllocationDisabled = errors.New("IPv6 egress allocation secret is not configured")
	ErrPoolInUse          = errors.New("IPv6 egress pool still has account bindings")
	ErrRuntimeUnavailable = errors.New("IPv6 egress runtime is not ready")
	ErrAutoConfigure      = errors.New("IPv6 egress automatic configuration failed")
)

type Pool struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	CIDR              string     `json:"cidr"`
	NodeID            *string    `json:"node_id,omitempty"`
	Status            string     `json:"status"`
	IsDefault         bool       `json:"is_default"`
	AllocationVersion int64      `json:"allocation_version"`
	AllocatedCount    int64      `json:"allocated_count"`
	Capacity          string     `json:"capacity"`
	RouteHealthy      *bool      `json:"route_healthy,omitempty"`
	LastProbeAt       *time.Time `json:"last_probe_at,omitempty"`
	ProbeError        string     `json:"probe_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Binding struct {
	ID          int64      `json:"id"`
	AccountID   int64      `json:"account_id"`
	AccountName string     `json:"account_name,omitempty"`
	PoolID      int64      `json:"pool_id"`
	PoolName    string     `json:"pool_name,omitempty"`
	PoolStatus  string     `json:"pool_status,omitempty"`
	SourceIPv6  string     `json:"source_ipv6"`
	Status      string     `json:"status"`
	Version     int64      `json:"version"`
	RotatedAt   *time.Time `json:"rotated_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (b *Binding) Route(inherited bool) platformegress.Route {
	if b == nil || b.Status != BindingStatusActive || b.PoolStatus != PoolStatusActive {
		return platformegress.Route{Mode: platformegress.ModeIPv6Pool, Inherited: inherited}
	}
	return platformegress.IPv6PoolRoute(b.SourceIPv6, b.PoolID, b.Version, inherited)
}

type CreatePoolInput struct {
	Name      string  `json:"name"`
	CIDR      string  `json:"cidr"`
	NodeID    *string `json:"node_id"`
	IsDefault bool    `json:"is_default"`
}

type UpdatePoolInput struct {
	Name      *string `json:"name"`
	NodeID    *string `json:"node_id"`
	Status    *string `json:"status"`
	IsDefault *bool   `json:"is_default"`
}

type SetAccountRouteInput struct {
	Mode   platformegress.Mode `json:"mode"`
	PoolID *int64              `json:"pool_id"`
}

type BindingPage struct {
	Items []Binding `json:"items"`
	Total int64     `json:"total"`
}

type AutoConfigureResult struct {
	Enabled  bool                               `json:"enabled"`
	Created  bool                               `json:"created"`
	Detected platformegress.DetectedIPv6Network `json:"detected"`
	Pool     *Pool                              `json:"pool"`
	Probe    *platformegress.ProbeResult        `json:"probe"`
}

type Store interface {
	CreatePool(ctx context.Context, input CreatePoolInput) (*Pool, error)
	UpdatePool(ctx context.Context, id int64, input UpdatePoolInput) (*Pool, error)
	DeletePool(ctx context.Context, id int64) error
	GetPool(ctx context.Context, id int64) (*Pool, error)
	GetDefaultPool(ctx context.Context) (*Pool, error)
	ListPools(ctx context.Context) ([]Pool, error)

	GetBinding(ctx context.Context, accountID int64) (*Binding, error)
	GetAnyBindingForPool(ctx context.Context, poolID int64) (*Binding, error)
	ListBindings(ctx context.Context, offset, limit int, search string) (*BindingPage, error)
	UpsertBinding(ctx context.Context, binding Binding, expectedVersion *int64) (*Binding, error)
	DeleteBinding(ctx context.Context, accountID int64) error
	SetAccountMode(ctx context.Context, accountID int64, mode platformegress.Mode) error
	ListInheritedAccountIDsWithoutBinding(ctx context.Context, limit int) ([]int64, error)
}
