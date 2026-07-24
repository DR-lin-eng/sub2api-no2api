package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const (
	usageBillingOverlayRecoveryLockKey = "billing:usage:overlay-recovery-lock"
	usageBillingOverlayRecoveryLockTTL = 5 * time.Minute
	usageBillingOverlayRecoveryBatch   = 256
)

var usageBillingOverlayRecoveryUnlockScript = redis.NewScript(`
	if redis.call('GET', KEYS[1]) == ARGV[1] then
		return redis.call('DEL', KEYS[1])
	end
	return 0
`)

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
		usageBillingPendingActualCost(cmd),
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay failed", "request_id", cmd.RequestID, "error", err)
	}
}

func (r *queuedUsageBillingRepository) recoverPendingOverlays() {
	if r == nil || r.rdb == nil || r.db == nil {
		return
	}
	timeout := max(2*time.Minute, r.commandTimeout*2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("skip durable usage billing Redis overlay recovery", "error", err)
		return
	}
	lockTokenBytes := make([]byte, 16)
	if _, err := rand.Read(lockTokenBytes); err != nil {
		slog.Warn("create durable usage billing overlay recovery lock failed", "error", err)
		return
	}
	lockToken := hex.EncodeToString(lockTokenBytes)
	locked, err := r.rdb.SetNX(ctx, usageBillingOverlayRecoveryLockKey, lockToken, usageBillingOverlayRecoveryLockTTL).Result()
	if err != nil {
		slog.Warn("lock durable usage billing overlay recovery failed", "error", err)
		return
	}
	if !locked {
		return
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), time.Second)
		defer unlockCancel()
		if _, unlockErr := usageBillingOverlayRecoveryUnlockScript.Run(unlockCtx, r.rdb, []string{usageBillingOverlayRecoveryLockKey}, lockToken).Result(); unlockErr != nil {
			slog.Warn("unlock durable usage billing overlay recovery failed", "error", unlockErr)
		}
	}()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Warn("begin durable usage billing overlay recovery failed", "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize the short startup rebuild with queue inserts and settlements so
	// the Redis projection exactly matches one PostgreSQL queue snapshot.
	if _, err := tx.ExecContext(ctx, "LOCK TABLE usage_billing_jobs IN SHARE MODE"); err != nil {
		slog.Warn("lock durable usage billing jobs for overlay recovery failed", "error", err)
		return
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT payload
		FROM usage_billing_jobs
		WHERE settled_at IS NULL
		ORDER BY id
	`)
	if err != nil {
		slog.Warn("query durable usage billing overlays failed", "error", err)
		return
	}
	pending := make(map[string]float64)
	markers := make(map[string]string)
	recovered := 0
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
		addPendingUsageBillingCosts(pending, &cmd)
		markers[usageBillingOverlayKey(cmd.RequestID, cmd.APIKeyID)] = cmd.RequestFingerprint
		recovered++
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterate durable usage billing overlays failed", "error", err)
		return
	}
	if err := rows.Close(); err != nil {
		slog.Warn("close durable usage billing overlay rows failed", "error", err)
		return
	}
	if err := r.replacePendingOverlayState(ctx, pending, markers); err != nil {
		slog.Warn("recover durable usage billing Redis overlays failed", "error", err)
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Warn("commit durable usage billing overlay recovery failed", "error", err)
		return
	}
	if recovered > 0 {
		slog.Info("durable usage billing Redis overlays recovered", "jobs", recovered)
	}
}

func addPendingUsageBillingCosts(pending map[string]float64, cmd *service.UsageBillingCommand) {
	if cmd.BalanceCost > 0 {
		pending[usageBillingPendingBalanceKey(cmd.UserID)] += cmd.BalanceCost
	}
	if cmd.SubscriptionCost > 0 {
		pending[usageBillingPendingSubscriptionKey(cmd.UserID, cmd.GroupID)] += cmd.SubscriptionCost
	}
	if cmd.APIKeyQuotaCost > 0 {
		pending[usageBillingPendingAPIKeyQuotaKey(cmd.APIKeyID)] += cmd.APIKeyQuotaCost
	}
	if cmd.APIKeyRateLimitCost > 0 {
		pending[usageBillingPendingAPIKeyRateLimitKey(cmd.APIKeyID)] += cmd.APIKeyRateLimitCost
	}
	if cost := usageBillingPendingActualCost(cmd); cost > 0 {
		pending[usageBillingPendingAPIKeyUsageKey(cmd.APIKeyID)] += cost
	}
}

func (r *queuedUsageBillingRepository) replacePendingOverlayState(ctx context.Context, pending map[string]float64, markers map[string]string) error {
	existingPending, err := r.scanUsageBillingRedisKeys(ctx, usageBillingPendingPrefix+"*")
	if err != nil {
		return err
	}
	existingMarkers, err := r.scanUsageBillingRedisKeys(ctx, usageBillingOverlayPrefix+"*")
	if err != nil {
		return err
	}

	pipe := r.rdb.Pipeline()
	queued := 0
	flush := func() error {
		if queued == 0 {
			return nil
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		pipe = r.rdb.Pipeline()
		queued = 0
		return nil
	}
	queue := func() error {
		queued++
		if queued >= usageBillingOverlayRecoveryBatch {
			return flush()
		}
		return nil
	}

	seenPending := make(map[string]struct{}, len(existingPending))
	for _, key := range existingPending {
		seenPending[key] = struct{}{}
		if amount, ok := pending[key]; ok && amount > 0 {
			pipe.Set(ctx, key, strconv.FormatFloat(amount, 'g', -1, 64), 0)
		} else {
			pipe.Del(ctx, key)
		}
		if err := queue(); err != nil {
			return err
		}
	}
	for key, amount := range pending {
		if _, ok := seenPending[key]; ok || amount <= 0 {
			continue
		}
		pipe.Set(ctx, key, strconv.FormatFloat(amount, 'g', -1, 64), 0)
		if err := queue(); err != nil {
			return err
		}
	}

	seenMarkers := make(map[string]struct{}, len(existingMarkers))
	for _, key := range existingMarkers {
		seenMarkers[key] = struct{}{}
		if fingerprint, ok := markers[key]; ok {
			pipe.Set(ctx, key, fingerprint, 0)
		} else {
			pipe.Del(ctx, key)
		}
		if err := queue(); err != nil {
			return err
		}
	}
	for key, fingerprint := range markers {
		if _, ok := seenMarkers[key]; ok {
			continue
		}
		pipe.Set(ctx, key, fingerprint, 0)
		if err := queue(); err != nil {
			return err
		}
	}
	return flush()
}

func (r *queuedUsageBillingRepository) scanUsageBillingRedisKeys(ctx context.Context, pattern string) ([]string, error) {
	keys := make([]string, 0)
	var cursor uint64
	for {
		batch, next, err := r.rdb.Scan(ctx, cursor, pattern, usageBillingOverlayRecoveryBatch).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			return keys, nil
		}
	}
}

func (r *queuedUsageBillingRepository) completePendingOverlay(cmd *service.UsageBillingCommand) error {
	if r == nil || r.rdb == nil || cmd == nil {
		return nil
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
		usageBillingPendingActualCost(cmd),
	).Result(); err != nil {
		slog.Warn("durable usage billing Redis overlay completion failed", "request_id", cmd.RequestID, "error", err)
		return err
	}
	if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
		pipe := r.rdb.Pipeline()
		pipe.Del(ctx, apiKeyAuthCacheKey(cmd.APIKeyAuthCacheKey))
		pipe.Publish(ctx, authCacheInvalidateChannel, cmd.APIKeyAuthCacheKey)
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Warn("durable usage billing API key cache invalidation failed", "request_id", cmd.RequestID, "error", err)
			return err
		}
	}
	return nil
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
		usageBillingPendingAPIKeyUsageKey(apiKeyID),
	}
}

func usageBillingPendingActualCost(cmd *service.UsageBillingCommand) float64 {
	if cmd == nil {
		return 0
	}
	return max(0, cmd.BalanceCost, cmd.SubscriptionCost, cmd.APIKeyQuotaCost, cmd.APIKeyRateLimitCost)
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

func usageBillingPendingAPIKeyUsageKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-usage:" + strconv.FormatInt(apiKeyID, 10)
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
