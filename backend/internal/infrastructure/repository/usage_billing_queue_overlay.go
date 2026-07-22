package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *queuedUsageBillingRepository) wakeConsumers() {
	for i := 0; i < r.consumerCount; i++ {
		select {
		case r.wakeCh <- struct{}{}:
		default:
			return
		}
	}
}

func (r *queuedUsageBillingRepository) reconcilePendingOverlay(cmd *service.UsageBillingCommand) {
	if r == nil || r.rdb == nil || cmd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	keys := usageBillingRedisKeys(cmd)
	if _, err := usageBillingOverlayScript.Run(ctx, r.rdb, keys,
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		int64(usageBillingMutationTTL/time.Second),
		cmd.RequestFingerprint,
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay failed", "request_id", cmd.RequestID, "error", err)
	}
}

func (r *queuedUsageBillingRepository) recoverPendingOverlays() {
	if r == nil || r.rdb == nil || r.db == nil {
		return
	}
	timeout := max(30*time.Second, r.commandTimeout*2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("skip durable usage billing Redis overlay recovery", "error", err)
		return
	}
	if _, err := usageBillingOverlayScript.Load(ctx, r.rdb).Result(); err != nil {
		slog.Warn("load durable usage billing Redis overlay script failed", "error", err)
		return
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT payload
		FROM usage_billing_jobs
		ORDER BY id
	`)
	if err != nil {
		slog.Warn("query durable usage billing overlays failed", "error", err)
		return
	}
	defer func() { _ = rows.Close() }()

	const pipelineSize = 256
	pipe := r.rdb.Pipeline()
	queued := 0
	recovered := 0
	flush := func() bool {
		if queued == 0 {
			return true
		}
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			slog.Warn("recover durable usage billing Redis overlays failed", "error", execErr)
			return false
		}
		recovered += queued
		queued = 0
		pipe = r.rdb.Pipeline()
		return true
	}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			slog.Warn("scan durable usage billing overlay failed", "error", err)
			return
		}
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal(payload, &cmd); err != nil {
			continue
		}
		cmd.Normalize()
		pipe.EvalSha(ctx, usageBillingOverlayScript.Hash(), usageBillingRedisKeys(&cmd),
			cmd.BalanceCost,
			cmd.SubscriptionCost,
			cmd.APIKeyQuotaCost,
			cmd.APIKeyRateLimitCost,
			int64(usageBillingMutationTTL/time.Second),
			cmd.RequestFingerprint,
		)
		queued++
		if queued >= pipelineSize && !flush() {
			return
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterate durable usage billing overlays failed", "error", err)
		return
	}
	if !flush() {
		return
	}
	if recovered > 0 {
		slog.Info("durable usage billing Redis overlays recovered", "jobs", recovered)
	}
}

func (r *queuedUsageBillingRepository) completePendingOverlay(cmd *service.UsageBillingCommand) {
	if r == nil || r.rdb == nil || cmd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	keys := usageBillingRedisKeys(cmd)
	if _, err := usageBillingCompleteOverlayScript.Run(ctx, r.rdb, keys,
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		int64(usageBillingMutationTTL/time.Second),
		cmd.RequestFingerprint,
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay completion failed", "request_id", cmd.RequestID, "error", err)
	}
	if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
		pipe := r.rdb.Pipeline()
		pipe.Del(ctx, apiKeyAuthCacheKey(cmd.APIKeyAuthCacheKey))
		pipe.Publish(ctx, authCacheInvalidateChannel, cmd.APIKeyAuthCacheKey)
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Warn("durable usage billing API key cache invalidation failed", "request_id", cmd.RequestID, "error", err)
		}
	}
}

func usageBillingRedisKeys(cmd *service.UsageBillingCommand) []string {
	userID, groupID, apiKeyID := int64(0), int64(0), int64(0)
	requestID := "invalid"
	if cmd != nil {
		userID, groupID, apiKeyID = cmd.UserID, cmd.GroupID, cmd.APIKeyID
		requestID = cmd.RequestID
	}
	return []string{
		"billing:usage:durable",
		usageBillingOverlayKey(requestID, apiKeyID),
		usageBillingPendingBalanceKey(userID),
		usageBillingPendingSubscriptionKey(userID, groupID),
		usageBillingPendingAPIKeyQuotaKey(apiKeyID),
		usageBillingPendingAPIKeyRateLimitKey(apiKeyID),
		billingBalanceKey(userID),
		billingSubKey(userID, groupID),
		billingRateLimitKey(apiKeyID),
		usageBillingBalanceMutationKey(userID),
		usageBillingSubscriptionMutationKey(userID, groupID),
		usageBillingAPIKeyRateLimitMutationKey(apiKeyID),
		usageBillingOverlayKey(requestID, apiKeyID),
	}
}

func usageBillingRequestKey(requestID string, apiKeyID int64) string {
	return strings.TrimSpace(requestID) + "\x00" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingOverlayKey(requestID string, apiKeyID int64) string {
	sum := sha256.Sum256([]byte(usageBillingRequestKey(requestID, apiKeyID)))
	return usageBillingOverlayPrefix + hex.EncodeToString(sum[:])
}

func usageBillingPendingBalanceKey(userID int64) string {
	return usageBillingPendingPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingPendingSubscriptionKey(userID, groupID int64) string {
	return usageBillingPendingPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingPendingAPIKeyQuotaKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-quota:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingPendingAPIKeyRateLimitKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingBalanceMutationKey(userID int64) string {
	return usageBillingMutationPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingSubscriptionMutationKey(userID, groupID int64) string {
	return usageBillingMutationPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingAPIKeyRateLimitMutationKey(apiKeyID int64) string {
	return usageBillingMutationPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func isPermanentUsageBillingError(err error) bool {
	return errors.Is(err, errUsageBillingJobPayloadInvalid) ||
		errors.Is(err, service.ErrUsageBillingRequestConflict) ||
		errors.Is(err, service.ErrUsageBillingRequestIDRequired) ||
		errors.Is(err, service.ErrUserNotFound) ||
		errors.Is(err, service.ErrAPIKeyNotFound) ||
		errors.Is(err, service.ErrAccountNotFound) ||
		errors.Is(err, service.ErrSubscriptionNotFound)
}
