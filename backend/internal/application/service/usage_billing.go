package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/shopspring/decimal"
)

var ErrUsageBillingRequestIDRequired = errors.New("usage billing request_id is required")
var ErrUsageBillingRequestConflict = errors.New("usage billing request fingerprint conflict")

// 通用余额冻结（reserve/capture/release）错误，供非 BatchImage 的异步任务复用。
var (
	ErrBalanceHoldInsufficientBalance = infraerrors.New(http.StatusPaymentRequired, "BALANCE_HOLD_INSUFFICIENT_BALANCE", "insufficient balance for balance hold")
	ErrBalanceHoldCostExceedsHold     = infraerrors.New(http.StatusConflict, "BALANCE_HOLD_COST_EXCEEDS_HOLD", "settlement cost exceeds held balance")
	ErrBalanceHoldFailed              = infraerrors.New(http.StatusBadGateway, "BALANCE_HOLD_FAILED", "balance hold failed")
	ErrBalanceHoldNotFound            = infraerrors.New(http.StatusNotFound, "BALANCE_HOLD_NOT_FOUND", "balance hold not found")
	// 视频单价既没有分组配置价、模型也没有内置每秒价时拒绝计费，不做任何价格猜测。
	ErrVideoPriceNotConfigured = infraerrors.New(http.StatusServiceUnavailable, "VIDEO_PRICE_NOT_CONFIGURED", "video price is not configured for this model and resolution")
	// 图片单价既没有分组配置价、模型也没有显式定价时拒绝计费，不做任何价格猜测。
	ErrImagePriceNotConfigured = infraerrors.New(http.StatusServiceUnavailable, "IMAGE_PRICE_NOT_CONFIGURED", "image price is not configured for this model and size")
)

const (
	balanceHoldRequestPrefix    = "balance_hold:"
	balanceCaptureRequestPrefix = "balance_capture:"
	balanceReleaseRequestPrefix = "balance_release:"
)

func BalanceHoldRequestID(holdKey string) string {
	return balanceHoldRequestPrefix + strings.TrimSpace(holdKey)
}

func BalanceCaptureRequestID(holdKey string) string {
	return balanceCaptureRequestPrefix + strings.TrimSpace(holdKey)
}

func BalanceReleaseRequestID(holdKey string) string {
	return balanceReleaseRequestPrefix + strings.TrimSpace(holdKey)
}

// UsageBillingCommand describes one billable request that must be applied at most once.
type UsageBillingCommand struct {
	RequestID          string
	APIKeyID           int64
	RequestFingerprint string
	RequestPayloadHash string
	APIKeyAuthCacheKey string

	UserID              int64
	AccountID           int64
	GroupID             int64
	SubscriptionID      *int64
	AccountType         string
	Model               string
	ServiceTier         string
	ReasoningEffort     string
	BillingType         int8
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	ImageCount          int
	MediaType           string

	BalanceCost         float64
	SubscriptionCost    float64
	APIKeyQuotaCost     float64
	APIKeyRateLimitCost float64
	AccountQuotaCost    float64
	// SQL selection hints only; excluded from the billing identity/fingerprint.
	AccountQuotaHasDailyLimit  bool
	AccountQuotaHasWeeklyLimit bool

	QuotaPlatform         string
	UserPlatformQuotaCost float64
}

func (c *UsageBillingCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildUsageBillingFingerprint(c)
	}
	// Preserve the raw-value fingerprint across upgrades, then normalize every
	// amount written to a NUMERIC(20,8) balance or quota column.
	c.quantizeMonetaryFields()
}

const UsageBillingMonetaryScale = 8

func (c *UsageBillingCommand) quantizeMonetaryFields() {
	c.BalanceCost = QuantizeUsageBillingAmount(c.BalanceCost)
	c.SubscriptionCost = QuantizeUsageBillingAmount(c.SubscriptionCost)
	c.APIKeyQuotaCost = QuantizeUsageBillingAmount(c.APIKeyQuotaCost)
	c.APIKeyRateLimitCost = QuantizeUsageBillingAmount(c.APIKeyRateLimitCost)
	c.AccountQuotaCost = QuantizeUsageBillingAmount(c.AccountQuotaCost)
	c.UserPlatformQuotaCost = QuantizeUsageBillingAmount(c.UserPlatformQuotaCost)
}

// QuantizeUsageBillingAmount matches PostgreSQL NUMERIC half-away-from-zero
// rounding without introducing a binary multiply/divide boundary error.
func QuantizeUsageBillingAmount(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	quantized, _ := decimal.NewFromFloat(v).Round(UsageBillingMonetaryScale).Float64()
	return quantized
}

func buildUsageBillingFingerprint(c *UsageBillingCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%s|%d|%0.10f|%0.10f|%0.10f|%0.10f|%0.10f",
		c.UserID,
		c.AccountID,
		c.APIKeyID,
		strings.TrimSpace(c.AccountType),
		strings.TrimSpace(c.Model),
		strings.TrimSpace(c.ServiceTier),
		strings.TrimSpace(c.ReasoningEffort),
		c.BillingType,
		c.InputTokens,
		c.OutputTokens,
		c.CacheCreationTokens,
		c.CacheReadTokens,
		c.ImageCount,
		strings.TrimSpace(c.MediaType),
		valueOrZero(c.SubscriptionID),
		c.BalanceCost,
		c.SubscriptionCost,
		c.APIKeyQuotaCost,
		c.APIKeyRateLimitCost,
		c.AccountQuotaCost,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	if platform := strings.TrimSpace(c.QuotaPlatform); platform != "" || c.UserPlatformQuotaCost > 0 {
		raw += fmt.Sprintf("|%s|%0.10f", platform, c.UserPlatformQuotaCost)
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func HashUsageRequestPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func HashAPIKeyAuthCacheKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// AccountQuotaState holds the post-increment quota state returned by the DB transaction.
// All values are post-update (i.e., already include the increment).
type AccountQuotaState struct {
	TotalUsed   float64
	TotalLimit  float64
	DailyUsed   float64
	DailyLimit  float64
	WeeklyUsed  float64
	WeeklyLimit float64
}

type UsageBillingApplyResult struct {
	Applied              bool
	Deferred             bool // true when repository owns Redis cache synchronization; callers must not apply cache deltas
	APIKeyQuotaExhausted bool
	NewBalance           *float64           // post-deduction balance (nil = no balance deduction)
	BalanceOverdrafted   bool               // true when the sufficient-balance guard missed and debt was still recorded
	QuotaState           *AccountQuotaState // post-increment quota state (nil = no quota increment)
}

// BatchImageBalanceHoldCommand describes an idempotent balance hold operation.
type BatchImageBalanceHoldCommand struct {
	RequestID          string
	APIKeyID           int64
	RequestFingerprint string
	RequestPayloadHash string
	UserID             int64
	BatchID            string
	HoldAmount         float64
	ActualAmount       float64
}

func (c *BatchImageBalanceHoldCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.BatchID = strings.TrimSpace(c.BatchID)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildBatchImageBalanceHoldFingerprint(c)
	}
}

func buildBatchImageBalanceHoldFingerprint(c *BatchImageBalanceHoldCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%s|%0.10f|%0.10f",
		c.UserID,
		c.APIKeyID,
		strings.TrimSpace(c.BatchID),
		c.HoldAmount,
		c.ActualAmount,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type BatchImageBalanceHoldResult struct {
	Applied       bool
	NewBalance    *float64
	FrozenBalance *float64
}

// BalanceHoldCommand 描述一次幂等的余额冻结操作（预留/结算/释放）。
// 与 BatchImageBalanceHoldCommand 的区别仅在 HoldKey 的语义：BatchImage 用 batch id，
// 其他异步任务（例如多模态视频生成）用自己生成的一次性 hold key。
type BalanceHoldCommand struct {
	RequestID          string
	APIKeyID           int64
	RequestFingerprint string
	RequestPayloadHash string
	UserID             int64
	HoldKey            string
	HoldAmount         float64
	ActualAmount       float64
}

func (c *BalanceHoldCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.HoldKey = strings.TrimSpace(c.HoldKey)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildBalanceHoldFingerprint(c)
	}
}

func buildBalanceHoldFingerprint(c *BalanceHoldCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf("%d|%d|%s|%0.10f|%0.10f",
		c.UserID,
		c.APIKeyID,
		strings.TrimSpace(c.HoldKey),
		c.HoldAmount,
		c.ActualAmount,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type BalanceHoldResult struct {
	Applied       bool
	NewBalance    *float64
	FrozenBalance *float64
}

// 余额冻结记录状态。pending 表示金额仍在 frozen_balance 中等待结算或释放。
const (
	BalanceHoldStatusPending  = "pending"
	BalanceHoldStatusSettled  = "settled"
	BalanceHoldStatusReleased = "released"
)

// BalanceHoldRecord 是一次余额冻结的持久化快照。
// 金额本身由 users.frozen_balance 承载，这里只记录冻结上下文，用于：
//   - 轮询阶段按上游任务 id 反查冻结金额、模型、分辨率；
//   - 超时扫描找到需要释放的冻结；
//   - 记录最终结算金额，便于回显与对账。
type BalanceHoldRecord struct {
	HoldKey           string
	RequestID         string
	UpstreamRequestID string
	UserID            int64
	APIKeyID          int64
	GroupID           int64
	AccountID         int64
	Model             string
	Resolution        string
	DurationSeconds   int
	HoldAmount        float64
	ActualAmount      *float64
	Currency          string
	Status            string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error)
	ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	ReserveBalance(ctx context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error)
	CaptureBalance(ctx context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error)
	ReleaseBalance(ctx context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error)
	CreateBalanceHold(ctx context.Context, record *BalanceHoldRecord) error
	BindBalanceHoldUpstreamRequest(ctx context.Context, holdKey, upstreamRequestID string) error
	GetBalanceHoldByUpstreamRequestID(ctx context.Context, upstreamRequestID string) (*BalanceHoldRecord, error)
	UpdateBalanceHoldStatus(ctx context.Context, holdKey, status string, actualAmount *float64) error
	ListExpiredBalanceHolds(ctx context.Context, now time.Time, limit int) ([]*BalanceHoldRecord, error)
}
