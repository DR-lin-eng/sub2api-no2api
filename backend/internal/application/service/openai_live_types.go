package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	LiveControllerPending  = "pending"
	LiveControllerObserver = "observer"
	LiveControllerProxy    = "proxy"
	LiveControllerClosed   = "closed"
)

var (
	ErrLiveUnavailable       = errors.New("live is unavailable")
	ErrLiveConcurrencyFull   = errors.New("live concurrency is full")
	ErrLiveCallNotFound      = errors.New("live call not found")
	ErrLiveIdentityMismatch  = errors.New("live call identity mismatch")
	ErrLiveControllerChanged = errors.New("live controller changed")
)

type LiveAttestationUnavailableError struct {
	Reason string
}

func (e *LiveAttestationUnavailableError) Error() string {
	if e == nil || e.Reason == "" {
		return "Live attestation is unavailable"
	}
	return "Live attestation is unavailable: " + e.Reason
}

// LiveCallRequest 是两个下游创建协议归一后的请求。Session 不做结构改写。
type LiveCallRequest struct {
	SDP     string          `json:"sdp"`
	Session json.RawMessage `json:"session"`
}

type LiveCallIdentity struct {
	APIKeyID        int64
	UserID          int64
	GroupID         *int64
	SubscriptionID  *int64
	UserAgent       string
	IPAddress       string
	InboundEndpoint string
	Billing         *LiveBillingSnapshot
}

type LiveCallRecord struct {
	CallID            string
	CallHash          string
	AccountID         int64
	APIKeyID          int64
	UserID            int64
	GroupID           int64
	SubscriptionID    int64
	LeaseID           string
	Model             string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	Controller        string
	ControllerOwner   string
	UserAgent         string
	IPAddress         string
	InboundEndpoint   string
	CodexVoice        bool
	SessionID         string
	ThreadID          string
	Billing           *LiveBillingSnapshot
	BillingDurationMs int64
	// AttestationCiphertext 仅用于让同一会话的 Sideband 复用创建时的证明。
	AttestationCiphertext string
}

type LiveCallCreated struct {
	SDP       []byte
	CallID    string
	Location  string
	Account   *Account
	SessionID string
	ThreadID  string
}

// LiveBillingSnapshot contains only the immutable monetary and quota policy
// needed to settle a call after disconnect or a process restart. No API secret
// or account credential is stored here.
type LiveBillingSnapshot struct {
	RateMultiplier        float64
	AccountRateMultiplier float64
	APIKeyQuota           bool
	APIKeyRateLimit       bool
	APIKeyAuthCacheKey    string
	UserPlatformQuota     bool
}

// LiveCallBillingStore freezes the terminal duration and keeps unfinished
// settlements discoverable across nodes until both billing and usage persist.
type LiveCallBillingStore interface {
	PrepareLiveCallSettlement(context.Context, string, time.Time) (*LiveCallRecord, error)
	ListDueLiveCalls(context.Context, time.Time, int) ([]*LiveCallRecord, error)
	MarkLiveCallBilled(context.Context, string, time.Duration) error
}

// LiveConcurrencyReplacements identifies the short-lived request slots held
// while a Live lease is installed. Each allowance is scoped independently so
// standalone user/API-key slots are never mistaken for Redis-backed slots.
type LiveConcurrencyReplacements struct {
	Account bool
	User    bool
	APIKey  bool
}

// LiveCallStore 由 GatewayCache 的 Redis 实现可选提供，避免扩大旧缓存接口。
type LiveCallStore interface {
	SaveLiveCall(ctx context.Context, record *LiveCallRecord, ttl time.Duration) error
	GetLiveCall(ctx context.Context, callHash string) (*LiveCallRecord, error)
	ClaimLiveController(ctx context.Context, callHash, controller, owner string) (bool, error)
	ReleaseLiveController(ctx context.Context, callHash, owner string) (bool, error)
	GetLiveController(ctx context.Context, callHash string) (string, error)
	MarkLiveCallClosed(ctx context.Context, callHash string, ttl time.Duration) (bool, error)
}

type LiveConcurrencyCache interface {
	AcquireLiveLease(
		ctx context.Context,
		accountID int64,
		accountMax int,
		userID int64,
		userMax int,
		apiKeyID int64,
		apiKeyMax int,
		leaseID string,
		replacements LiveConcurrencyReplacements,
	) (bool, error)
	RefreshLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) (bool, error)
	ReleaseLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) error
}
