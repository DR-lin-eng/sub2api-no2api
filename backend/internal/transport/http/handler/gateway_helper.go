package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

// claudeCodeValidator is a singleton validator for Claude Code client detection
var claudeCodeValidator = service.NewClaudeCodeValidator()

// SetClaudeCodeClientContext 检查请求是否来自 Claude Code 客户端，并设置到 context 中
// 返回更新后的 context
func SetClaudeCodeClientContext(c *gin.Context, body []byte, parsedReq *service.ParsedRequest) {
	if c == nil || c.Request == nil {
		return
	}
	ua := c.GetHeader("User-Agent")
	// Fast path：非 Claude CLI UA 直接判定 false，避免热路径二次 JSON 反序列化。
	if !claudeCodeValidator.ValidateUserAgent(ua) {
		ctx := service.SetClaudeCodeClient(c.Request.Context(), false)
		c.Request = c.Request.WithContext(ctx)
		return
	}

	isClaudeCode := false
	if !strings.Contains(c.Request.URL.Path, "messages") {
		// 与 Validate 行为一致：非 messages 路径 UA 命中即可视为 Claude Code 客户端。
		isClaudeCode = true
	} else {
		// 仅在确认为 Claude CLI 且 messages 路径时再做 body 解析。
		bodyMap := claudeCodeBodyMapFromParsedRequest(parsedReq)
		if bodyMap == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &bodyMap)
		}
		isClaudeCode = claudeCodeValidator.Validate(c.Request, bodyMap)
	}

	// 更新 request context
	ctx := service.SetClaudeCodeClient(c.Request.Context(), isClaudeCode)

	// 仅在确认为 Claude Code 客户端时提取版本号写入 context
	if isClaudeCode {
		if version := claudeCodeValidator.ExtractVersion(ua); version != "" {
			ctx = service.SetClaudeCodeVersion(ctx, version)
		}
	}

	c.Request = c.Request.WithContext(ctx)
}

func claudeCodeBodyMapFromParsedRequest(parsedReq *service.ParsedRequest) map[string]any {
	if parsedReq == nil {
		return nil
	}
	bodyMap := map[string]any{
		"model": parsedReq.Model,
	}
	if parsedReq.HasSystem {
		if system, ok := parsedReq.SystemValue(); ok {
			bodyMap["system"] = system
		} else {
			bodyMap["system"] = nil
		}
	}
	if parsedReq.MetadataUserID != "" {
		bodyMap["metadata"] = map[string]any{"user_id": parsedReq.MetadataUserID}
	}
	return bodyMap
}

// 并发槽位等待相关常量
//
// 性能优化说明：
// 原实现使用固定间隔（100ms）轮询并发槽位，存在以下问题：
// 1. 高并发时频繁轮询增加 Redis 压力
// 2. 固定间隔可能导致多个请求同时重试（惊群效应）
//
// 新实现使用指数退避 + 抖动算法：
// 1. 初始退避 100ms，每次乘以 1.5，最大 2s
// 2. 添加 ±20% 的随机抖动，分散重试时间点
// 3. 减少 Redis 压力，避免惊群效应
const (
	// maxConcurrencyWait 等待并发槽位的最大时间
	maxConcurrencyWait = 30 * time.Second
	// defaultPingInterval 流式响应等待时发送 ping 的默认间隔
	defaultPingInterval = 10 * time.Second
	// initialBackoff 初始退避时间
	initialBackoff = 100 * time.Millisecond
	// backoffMultiplier 退避时间乘数（指数退避）
	backoffMultiplier = 1.5
	// maxBackoff 最大退避时间
	maxBackoff = 2 * time.Second
)

// SSEPingFormat defines the format of SSE ping events for different platforms
type SSEPingFormat string

const ssePingFormatOverrideKey = "handler_sse_ping_format_override"

const priorityAdmissionPendingBytesKey = "handler_priority_admission_pending_bytes"

const (
	// SSEPingFormatClaude is the Claude/Anthropic SSE ping format
	SSEPingFormatClaude SSEPingFormat = "data: {\"type\": \"ping\"}\n\n"
	// SSEPingFormatNone indicates no ping should be sent (e.g., OpenAI has no ping spec)
	SSEPingFormatNone SSEPingFormat = ""
	// SSEPingFormatComment is an SSE comment ping for OpenAI/Codex CLI clients
	SSEPingFormatComment SSEPingFormat = ":\n\n"
	// SSEPingFormatOpenAIImages is the pseudo-stream heartbeat used by Images requests.
	SSEPingFormatOpenAIImages SSEPingFormat = "data: {}\n\n"
)

// ConcurrencyError represents a concurrency limit error with context
type ConcurrencyError struct {
	SlotType  string
	IsTimeout bool
}

func (e *ConcurrencyError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("timeout waiting for %s concurrency slot", e.SlotType)
	}
	return fmt.Sprintf("%s concurrency limit reached", e.SlotType)
}

type WaitQueueFullError struct {
	SlotType string
}

func shouldLogConcurrencyAcquireError(err error) bool {
	var queueFull *WaitQueueFullError
	return !errors.As(err, &queueFull)
}

func (e *WaitQueueFullError) Error() string {
	return "Too many pending requests, please retry later"
}

// ConcurrencyHelper provides common concurrency slot management for gateway handlers
type ConcurrencyHelper struct {
	concurrencyService *service.ConcurrencyService
	pingFormat         SSEPingFormat
	pingInterval       time.Duration
}

// NewConcurrencyHelper creates a new ConcurrencyHelper
func NewConcurrencyHelper(concurrencyService *service.ConcurrencyService, pingFormat SSEPingFormat, pingInterval time.Duration) *ConcurrencyHelper {
	if pingInterval <= 0 {
		pingInterval = defaultPingInterval
	}
	return &ConcurrencyHelper{
		concurrencyService: concurrencyService,
		pingFormat:         pingFormat,
		pingInterval:       pingInterval,
	}
}

func setSSEPingFormatOverride(c *gin.Context, format SSEPingFormat) {
	if c != nil {
		c.Set(ssePingFormatOverrideKey, format)
	}
}

func resolveSSEPingFormat(c *gin.Context, fallback SSEPingFormat) SSEPingFormat {
	if c == nil {
		return fallback
	}
	value, ok := c.Get(ssePingFormatOverrideKey)
	if !ok {
		return fallback
	}
	format, ok := value.(SSEPingFormat)
	if !ok {
		return fallback
	}
	return format
}

// SetPriorityAdmissionPendingBytes records the actual in-memory request body
// size after a gateway handler has buffered it. Callers that do not set it use
// Content-Length as a conservative compatibility fallback.
func SetPriorityAdmissionPendingBytes(c *gin.Context, size int64) {
	if c == nil {
		return
	}
	if size < 0 {
		size = 0
	}
	c.Set(priorityAdmissionPendingBytesKey, size)
}

// SetPriorityAdmissionPendingBytes records body memory only for requests that
// can enter the priority queues. The feature-off path therefore avoids a Gin
// context write and its possible map growth allocation.
func (h *ConcurrencyHelper) SetPriorityAdmissionPendingBytes(c *gin.Context, size int64) {
	if h == nil || h.concurrencyService == nil || c == nil || c.Request == nil || !h.concurrencyService.PriorityAdmissionEnabledForRequest(c.Request.Context()) {
		return
	}
	_, _, _ = h.priorityAdmissionRequestSnapshot(c)
	SetPriorityAdmissionPendingBytes(c, size)
}

func (h *ConcurrencyHelper) RefreshPriorityAdmissionRequestSnapshot(c *gin.Context) bool {
	if h == nil || h.concurrencyService == nil || c == nil || c.Request == nil {
		return false
	}
	tier := service.RequestSchedulingTierNormal
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		tier = service.NormalizeRequestSchedulingTier(subject.SchedulingTier)
	}
	ctx, enabled := h.concurrencyService.RefreshPriorityAdmissionRequestSnapshot(c.Request.Context(), tier)
	c.Request = c.Request.WithContext(ctx)
	SetPriorityAdmissionPendingBytes(c, 0)
	return enabled
}

// priorityAdmissionRequestSnapshot tags a gateway request only when priority
// admission is enabled as it enters the user-slot stage. Presence of the
// context value is the per-request snapshot used by the later scheduler and
// account-slot stage, including while an administrator toggles the feature.
func (h *ConcurrencyHelper) priorityAdmissionRequestSnapshot(c *gin.Context) (context.Context, service.RequestSchedulingTier, bool) {
	if c == nil || c.Request == nil {
		return context.Background(), service.RequestSchedulingTierNormal, false
	}
	ctx := c.Request.Context()
	if h == nil || h.concurrencyService == nil {
		return ctx, service.RequestSchedulingTierNormal, false
	}
	if tier, ok := service.RequestSchedulingTierFromContextOK(ctx); ok {
		ctx, enabled := h.concurrencyService.WithPriorityAdmissionRequestSnapshot(ctx, tier)
		if enabled {
			c.Request = c.Request.WithContext(ctx)
		}
		return ctx, tier, enabled
	}
	if !h.concurrencyService.PriorityAdmissionEnabled() {
		return ctx, service.RequestSchedulingTierNormal, false
	}
	tier := service.RequestSchedulingTierNormal
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		tier = service.NormalizeRequestSchedulingTier(subject.SchedulingTier)
	}
	var enabled bool
	ctx, enabled = h.concurrencyService.WithPriorityAdmissionRequestSnapshot(ctx, tier)
	if !enabled {
		return c.Request.Context(), service.RequestSchedulingTierNormal, false
	}
	c.Request = c.Request.WithContext(ctx)
	return ctx, tier, true
}

func priorityAdmissionPendingBytes(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	if value, ok := c.Get(priorityAdmissionPendingBytesKey); ok {
		switch size := value.(type) {
		case int64:
			if size > 0 {
				return size
			}
			return 0
		case int:
			if size > 0 {
				return int64(size)
			}
			return 0
		}
	}
	if c.Request != nil && c.Request.ContentLength > 0 {
		return c.Request.ContentLength
	}
	return 0
}

// wrapReleaseOnDone ensures release runs at most once and still triggers on context cancellation.
// 用于避免客户端断开或上游超时导致的并发槽位泄漏。
// 优化：基于 context.AfterFunc 注册回调，避免每请求额外守护 goroutine。
func wrapReleaseOnDone(ctx context.Context, releaseFunc func()) func() {
	if releaseFunc == nil {
		return nil
	}
	var once sync.Once
	releaseOnce := func() {
		once.Do(releaseFunc)
	}
	stop := context.AfterFunc(ctx, releaseOnce)

	return func() {
		_ = stop()
		releaseOnce()
	}
}

// IncrementWaitCount increments the wait count for a user
func (h *ConcurrencyHelper) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementWaitCount(ctx, userID, maxWait)
}

// DecrementWaitCount decrements the wait count for a user
func (h *ConcurrencyHelper) DecrementWaitCount(ctx context.Context, userID int64) {
	h.concurrencyService.DecrementWaitCount(ctx, userID)
}

// IncrementAccountWaitCount increments the wait count for an account
func (h *ConcurrencyHelper) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementAccountWaitCount(ctx, accountID, maxWait)
}

// DecrementAccountWaitCount decrements the wait count for an account
func (h *ConcurrencyHelper) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	h.concurrencyService.DecrementAccountWaitCount(ctx, accountID)
}

// TryAcquireUserSlot 尝试立即获取用户并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func (h *ConcurrencyHelper) TryAcquireUserSlotForAPIKey(ctx context.Context, userID int64, maxConcurrency int, apiKeyID int64, apiKeyMaxConcurrency int) (func(), bool, error) {
	releaseFunc, acquired, err := h.TryAcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil || !acquired {
		return releaseFunc, acquired, err
	}
	return h.withAPIKeySlot(ctx, apiKeyID, apiKeyMaxConcurrency, releaseFunc)
}

// AcquireOpenAIWSIngressLease bounds the whole client WebSocket lifecycle,
// independently from per-turn user and account slots.
func (h *ConcurrencyHelper) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int) (*service.OpenAIWSIngressLease, bool, error) {
	if h == nil || h.concurrencyService == nil {
		return nil, false, fmt.Errorf("concurrency service is unavailable")
	}
	return h.concurrencyService.AcquireOpenAIWSIngressLease(ctx, apiKeyID, maxConnections)
}

// TryAcquireAccountSlot 尝试立即获取账号并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

// AcquireUserSlotWithWait acquires a user concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireUserSlotWithWait(c *gin.Context, userID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.acquireUserSlotWithWaitTimeout(c, userID, maxConcurrency, maxConcurrencyWait, isStream, streamStarted)
}

func (h *ConcurrencyHelper) acquireUserSlotWithWaitTimeout(c *gin.Context, userID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	ctx, tier, priorityEnabled := h.priorityAdmissionRequestSnapshot(c)

	// Try to acquire immediately
	var releaseFunc func()
	var acquired bool
	var err error
	if priorityEnabled {
		result, acquireErr := h.concurrencyService.AcquireUserSlotForTier(ctx, userID, maxConcurrency, tier)
		if acquireErr != nil {
			return nil, acquireErr
		}
		acquired = result.Acquired
		releaseFunc = result.ReleaseFunc
	} else {
		releaseFunc, acquired, err = h.TryAcquireUserSlot(ctx, userID, maxConcurrency)
		if err != nil {
			return nil, err
		}
	}

	if acquired {
		return h.withAPIKeySlotFromGin(c, releaseFunc)
	}

	queueLimit := service.CalculateMaxWait(maxConcurrency) - maxConcurrency
	if queueLimit < 1 {
		queueLimit = 1
	}
	if priorityEnabled {
		waitLease, canWait, waitErr := h.concurrencyService.BeginPriorityUserWaitForContext(
			ctx,
			userID,
			maxConcurrency,
			queueLimit,
			tier,
			priorityAdmissionPendingBytes(c),
			timeout,
		)
		if waitErr != nil {
			return nil, waitErr
		}
		if !canWait {
			return nil, &WaitQueueFullError{SlotType: "user"}
		}
		defer waitLease.Close()

		priorityCtx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		releaseFunc, err = h.waitForPrioritySlotWithPing(
			priorityCtx,
			c,
			"user",
			isStream,
			streamStarted,
			waitLease.TryAcquire,
		)
		if err != nil {
			return nil, err
		}
		return h.withAPIKeySlotFromGin(c, releaseFunc)
	}
	canWait, err := h.IncrementWaitCount(ctx, userID, queueLimit)
	if err != nil {
		return nil, err
	}
	if !canWait {
		return nil, &WaitQueueFullError{SlotType: "user"}
	}
	defer h.DecrementWaitCount(ctx, userID)

	// Need to wait - handle streaming ping if needed
	releaseFunc, err = h.waitForSlotWithPingTimeout(c, "user", userID, maxConcurrency, timeout, isStream, streamStarted, false)
	if err != nil {
		return nil, err
	}
	return h.withAPIKeySlotFromGin(c, releaseFunc)
}

func (h *ConcurrencyHelper) withAPIKeySlotFromGin(c *gin.Context, releaseFunc func()) (func(), error) {
	if c == nil {
		return releaseFunc, nil
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		return releaseFunc, nil
	}
	combinedRelease, acquired, err := h.withAPIKeySlot(c.Request.Context(), apiKey.ID, apiKey.ConcurrencyLimit, releaseFunc)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, &ConcurrencyError{SlotType: "api key"}
	}
	return combinedRelease, nil
}

func (h *ConcurrencyHelper) withAPIKeySlot(ctx context.Context, apiKeyID int64, apiKeyMaxConcurrency int, releaseFunc func()) (func(), bool, error) {
	if h == nil || h.concurrencyService == nil || apiKeyID <= 0 {
		return releaseFunc, true, nil
	}
	result, err := h.concurrencyService.AcquireAPIKeySlot(ctx, apiKeyID, apiKeyMaxConcurrency)
	if err != nil {
		if releaseFunc != nil {
			releaseFunc()
		}
		return nil, false, err
	}
	if !result.Acquired {
		if releaseFunc != nil {
			releaseFunc()
		}
		return nil, false, nil
	}
	return func() {
		if releaseFunc != nil {
			releaseFunc()
		}
		if result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
	}, true, nil
}

// AcquireAccountSlotWithWait acquires an account concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireAccountSlotWithWait(c *gin.Context, accountID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	releaseFunc, acquired, err := h.TryAcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, err
	}

	if acquired {
		return releaseFunc, nil
	}

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "account", accountID, maxConcurrency, isStream, streamStarted)
}

// waitForSlotWithPing waits for a concurrency slot, sending ping events for streaming requests.
// streamStarted pointer is updated when streaming begins (for proper error handling by caller).
func (h *ConcurrencyHelper) waitForSlotWithPing(c *gin.Context, slotType string, id int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, slotType, id, maxConcurrency, maxConcurrencyWait, isStream, streamStarted, false)
}

// waitForSlotWithPingTimeout waits for a concurrency slot with a custom timeout.
func (h *ConcurrencyHelper) waitForSlotWithPingTimeout(c *gin.Context, slotType string, id int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool, tryImmediate bool) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	acquireSlot := func() (*service.AcquireResult, error) {
		if slotType == "user" {
			return h.concurrencyService.AcquireUserSlot(ctx, id, maxConcurrency)
		}
		return h.concurrencyService.AcquireAccountSlot(ctx, id, maxConcurrency)
	}

	if tryImmediate {
		result, err := acquireSlot()
		if err != nil {
			return nil, err
		}
		if result.Acquired {
			return result.ReleaseFunc, nil
		}
	}

	// Determine if ping is needed (streaming + ping format defined)
	pingFormat := resolveSSEPingFormat(c, h.pingFormat)
	needPing := isStream && pingFormat != ""

	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
	}

	// Only create ping ticker if ping is needed
	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(h.pingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}

	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			if parentErr := c.Request.Context().Err(); parentErr != nil {
				return nil, parentErr
			}
			return nil, &ConcurrencyError{
				SlotType:  slotType,
				IsTimeout: true,
			}

		case <-pingCh:
			// Send ping to keep connection alive
			if !*streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				*streamStarted = true
			}
			if _, err := fmt.Fprint(c.Writer, string(pingFormat)); err != nil {
				return nil, err
			}
			flusher.Flush()

		case <-timer.C:
			// Try to acquire slot
			result, err := acquireSlot()
			if err != nil {
				return nil, err
			}

			if result.Acquired {
				return result.ReleaseFunc, nil
			}
			backoff = nextBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}

// AcquireAccountSlotWithWaitTimeout acquires an account slot with a custom timeout (keeps SSE ping).
func (h *ConcurrencyHelper) AcquireAccountSlotWithWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, "account", accountID, maxConcurrency, timeout, isStream, streamStarted, true)
}

// AcquireAccountSlotWithPriorityWaitTimeout is the complete priority-aware
// account admission path. maxWaiting comes from the scheduler WaitPlan and
// pendingBytes must be the size of the already-buffered request body.
func (h *ConcurrencyHelper) AcquireAccountSlotWithPriorityWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, maxWaiting int, timeout time.Duration, pendingBytes int64, isStream bool, streamStarted *bool) (func(), error) {
	if h == nil || h.concurrencyService == nil {
		return nil, fmt.Errorf("concurrency service is unavailable")
	}
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("request context is unavailable")
	}
	requestCtx := c.Request.Context()
	tier, priorityEnabled := service.RequestSchedulingTierFromContextOK(requestCtx)
	if !priorityEnabled || !h.concurrencyService.PriorityAdmissionEnabledForRequest(requestCtx) {
		return h.AcquireAccountSlotWithWaitTimeout(c, accountID, maxConcurrency, timeout, isStream, streamStarted)
	}

	result, err := h.concurrencyService.AcquireAccountSlotForTier(requestCtx, accountID, maxConcurrency, tier)
	if err != nil {
		return nil, err
	}
	if result.Acquired {
		return result.ReleaseFunc, nil
	}
	if tier == service.RequestSchedulingTierLow {
		return nil, &WaitQueueFullError{SlotType: "account"}
	}

	ctx, cancel := context.WithTimeout(requestCtx, timeout)
	defer cancel()

	waiter, canWait, err := h.concurrencyService.BeginPriorityAccountWaitForContext(
		requestCtx,
		accountID,
		maxConcurrency,
		maxWaiting,
		tier,
		pendingBytes,
		timeout,
	)
	if err != nil {
		return nil, err
	}
	if !canWait {
		return nil, &WaitQueueFullError{SlotType: "account"}
	}
	defer waiter.Close()
	return h.waitForPrioritySlotWithPing(ctx, c, "account", isStream, streamStarted, waiter.TryAcquire)
}

type prioritySlotAcquireFunc func(context.Context) (*service.AcquireResult, service.PriorityAccountAdmissionStatus, error)

func (h *ConcurrencyHelper) waitForPrioritySlotWithPing(ctx context.Context, c *gin.Context, slotType string, isStream bool, streamStarted *bool, acquire prioritySlotAcquireFunc) (func(), error) {
	tryAcquire := func() (*service.AcquireResult, error) {
		result, status, err := acquire(ctx)
		if err != nil {
			return nil, err
		}
		if status == service.PriorityAccountAdmissionQueueFull || status == service.PriorityAccountAdmissionRejected {
			return nil, &WaitQueueFullError{SlotType: slotType}
		}
		if result == nil {
			return rejectedAcquireResultForHandler, nil
		}
		return result, nil
	}
	if result, err := tryAcquire(); err != nil {
		return nil, err
	} else if result.Acquired {
		return result.ReleaseFunc, nil
	}
	pingFormat := resolveSSEPingFormat(c, h.pingFormat)
	needPing := isStream && pingFormat != ""
	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
	}
	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(h.pingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}
	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			if parentErr := c.Request.Context().Err(); parentErr != nil {
				return nil, parentErr
			}
			return nil, &ConcurrencyError{SlotType: slotType, IsTimeout: true}
		case <-pingCh:
			if !*streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				*streamStarted = true
			}
			if _, writeErr := fmt.Fprint(c.Writer, string(pingFormat)); writeErr != nil {
				return nil, writeErr
			}
			flusher.Flush()
		case <-timer.C:
			acquired, acquireErr := tryAcquire()
			if acquireErr != nil {
				return nil, acquireErr
			}
			if acquired.Acquired {
				return acquired.ReleaseFunc, nil
			}
			backoff = nextBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}

var rejectedAcquireResultForHandler = &service.AcquireResult{}

// nextBackoff 计算下一次退避时间
// 性能优化：使用指数退避 + 随机抖动，避免惊群效应
// current: 当前退避时间
// 返回值：下一次退避时间（100ms ~ 2s 之间）
func nextBackoff(current time.Duration) time.Duration {
	// 指数退避：当前时间 * 1.5
	next := time.Duration(float64(current) * backoffMultiplier)
	if next > maxBackoff {
		next = maxBackoff
	}
	// 添加 ±20% 的随机抖动（jitter 范围 0.8 ~ 1.2）
	// 抖动可以分散多个请求的重试时间点，避免同时冲击 Redis
	jitter := 0.8 + rand.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < initialBackoff {
		return initialBackoff
	}
	if jittered > maxBackoff {
		return maxBackoff
	}
	return jittered
}
