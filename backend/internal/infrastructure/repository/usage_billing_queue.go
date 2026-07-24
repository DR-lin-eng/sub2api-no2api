package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/redis/go-redis/v9"
)

const (
	usageBillingPendingPrefix       = "billing:usage:pending:"
	usageBillingMutationPrefix      = "billing:usage:mutation:"
	usageBillingOverlayPrefix       = "billing:usage:overlay:"
	usageBillingEnqueueBatchMaxSize = 256
	usageBillingEnqueueBatchWindow  = 3 * time.Millisecond
	usageBillingEnqueueQueueSize    = 32768
	usageBillingMutationTTL         = 24 * time.Hour
)

var errUsageBillingJobPayloadInvalid = errors.New("usage billing job payload is invalid")

const usageBillingEnqueueBatchSQL = `
	WITH input AS (
		SELECT request_id, api_key_id, request_fingerprint, payload
		FROM jsonb_to_recordset($1::jsonb) AS x(
			request_id text,
			api_key_id bigint,
			request_fingerprint text,
			payload jsonb
		)
	),
	eligible AS (
		SELECT i.*
		FROM input i
		LEFT JOIN usage_billing_dedup d
			ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
		LEFT JOIN usage_billing_dedup_archive a
			ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
		WHERE d.id IS NULL AND a.request_id IS NULL
	),
	inserted AS (
		INSERT INTO usage_billing_jobs (
			request_id, api_key_id, request_fingerprint, payload
		)
		SELECT request_id, api_key_id, request_fingerprint, payload
		FROM eligible
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, request_id, api_key_id, request_fingerprint
	)
	SELECT
		i.request_id,
		i.api_key_id,
		COALESCE(ins.id, j.id, 0) AS job_id,
		CASE
			WHEN ins.id IS NOT NULL THEN 'inserted'
			WHEN j.id IS NOT NULL AND j.request_fingerprint = i.request_fingerprint THEN 'pending'
			WHEN j.id IS NOT NULL THEN 'conflict'
			WHEN d.id IS NOT NULL AND d.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN d.id IS NOT NULL THEN 'conflict'
			WHEN a.request_id IS NOT NULL AND a.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN a.request_id IS NOT NULL THEN 'conflict'
			ELSE 'conflict'
		END AS status
	FROM input i
	LEFT JOIN inserted ins
		ON ins.request_id = i.request_id AND ins.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_jobs j
		ON j.request_id = i.request_id AND j.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup d
		ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup_archive a
		ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
`

const usageBillingClaimBatchSQL = `
	WITH input AS (
		SELECT job_id, request_id, api_key_id, request_fingerprint
		FROM jsonb_to_recordset($1::jsonb) AS x(
			job_id bigint,
			request_id text,
			api_key_id bigint,
			request_fingerprint text
		)
	),
	eligible AS (
		SELECT i.*
		FROM input i
		LEFT JOIN usage_billing_dedup_archive a
			ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
		WHERE a.request_id IS NULL
	),
	inserted AS (
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		SELECT request_id, api_key_id, request_fingerprint
		FROM eligible
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING request_id, api_key_id, request_fingerprint
	)
	SELECT
		i.job_id,
		CASE
			WHEN ins.request_id IS NOT NULL THEN 'inserted'
			WHEN d.id IS NOT NULL AND d.request_fingerprint = i.request_fingerprint THEN 'applied'
			WHEN d.id IS NOT NULL THEN 'conflict'
			WHEN a.request_id IS NOT NULL AND a.request_fingerprint = i.request_fingerprint THEN 'applied'
			ELSE 'conflict'
		END AS status
	FROM input i
	LEFT JOIN inserted ins
		ON ins.request_id = i.request_id AND ins.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup d
		ON d.request_id = i.request_id AND d.api_key_id = i.api_key_id
	LEFT JOIN usage_billing_dedup_archive a
		ON a.request_id = i.request_id AND a.api_key_id = i.api_key_id
`

var usageBillingOverlayScript = redis.NewScript(`
	local existing = redis.call('GET', KEYS[13])
	if existing then
		if existing == ARGV[6] then
			return 0
		end
		return redis.error_reply('usage billing overlay fingerprint conflict')
	end

	redis.call('SET', KEYS[13], ARGV[6])
	local balance_cost = tonumber(ARGV[1]) or 0
	local subscription_cost = tonumber(ARGV[2]) or 0
	local api_quota_cost = tonumber(ARGV[3]) or 0
	local rate_limit_cost = tonumber(ARGV[4]) or 0
	local api_usage_cost = tonumber(ARGV[7]) or 0

	if balance_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[3], balance_cost)
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[5])
	end
	if subscription_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[4], subscription_cost)
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[5])
	end
	if api_quota_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[5], api_quota_cost)
	end
	if rate_limit_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[6], rate_limit_cost)
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[5])
	end
	if api_usage_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[14], api_usage_cost)
	end
	return 1
`)

var usageBillingCompleteOverlayScript = redis.NewScript(`
	local function subtract_pending(key, amount)
		if amount <= 0 then
			return
		end
		local current = tonumber(redis.call('GET', key) or '0')
		local updated = current - amount
		if updated <= 0.0000000001 then
			redis.call('DEL', key)
		else
			redis.call('SET', key, tostring(updated))
		end
	end

	local balance_cost = tonumber(ARGV[1]) or 0
	local subscription_cost = tonumber(ARGV[2]) or 0
	local api_quota_cost = tonumber(ARGV[3]) or 0
	local rate_limit_cost = tonumber(ARGV[4]) or 0
	local api_usage_cost = tonumber(ARGV[7]) or 0
	local marker = redis.call('GET', KEYS[13])
	if marker == ARGV[6] then
		subtract_pending(KEYS[3], balance_cost)
		subtract_pending(KEYS[4], subscription_cost)
		subtract_pending(KEYS[5], api_quota_cost)
		subtract_pending(KEYS[6], rate_limit_cost)
		subtract_pending(KEYS[14], api_usage_cost)
		redis.call('DEL', KEYS[13])
	end

	if balance_cost > 0 then
		redis.call('DEL', KEYS[7])
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[5])
	end
	if subscription_cost > 0 then
		redis.call('DEL', KEYS[8])
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[5])
	end
	if rate_limit_cost > 0 then
		redis.call('DEL', KEYS[9])
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[5])
	end
	return 1
`)

type usageBillingEnqueueStatus string

const (
	usageBillingEnqueueInserted usageBillingEnqueueStatus = "inserted"
	usageBillingEnqueuePending  usageBillingEnqueueStatus = "pending"
	usageBillingEnqueueApplied  usageBillingEnqueueStatus = "applied"
	usageBillingEnqueueConflict usageBillingEnqueueStatus = "conflict"
)

type usageBillingEnqueueRequest struct {
	cmd      service.UsageBillingCommand
	payload  json.RawMessage
	resultCh chan usageBillingEnqueueResult
}

type usageBillingEnqueueResult struct {
	status usageBillingEnqueueStatus
	jobID  int64
	err    error
}

type usageBillingBatchInput struct {
	RequestID          string          `json:"request_id"`
	APIKeyID           int64           `json:"api_key_id"`
	RequestFingerprint string          `json:"request_fingerprint"`
	Payload            json.RawMessage `json:"payload"`
}

type usageBillingClaimInput struct {
	JobID              int64  `json:"job_id"`
	RequestID          string `json:"request_id"`
	APIKeyID           int64  `json:"api_key_id"`
	RequestFingerprint string `json:"request_fingerprint"`
}

type usageBillingJob struct {
	id                 int64
	requestID          string
	apiKeyID           int64
	requestFingerprint string
	payload            []byte
	attempts           int
	createdAt          time.Time
}

type usageBillingCompletion struct {
	jobID int64
	cmd   service.UsageBillingCommand
}

type usageBillingPlatformQuotaAggregate struct {
	userID   int64
	platform string
	amount   float64
}

type queuedUsageBillingRepository struct {
	direct         *usageBillingRepository
	db             *sql.DB
	rdb            *redis.Client
	consumerCount  int
	readBatchSize  int
	pollInterval   time.Duration
	commandTimeout time.Duration
	maxRetryDelay  time.Duration

	enqueueCh chan usageBillingEnqueueRequest
	wakeCh    chan struct{}
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	stopped   atomic.Bool
	lifecycle sync.RWMutex
}

// ProvideUsageBillingRepository uses a PostgreSQL WAL-backed queue in
// production. Redis is optional and only accelerates pending-usage visibility.
func ProvideUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB, rdb *redis.Client, cfg *config.Config) service.UsageBillingRepository {
	direct := &usageBillingRepository{db: sqlDB}
	if cfg == nil || !cfg.Billing.Queue.Enabled || sqlDB == nil {
		return direct
	}
	queueCfg := cfg.Billing.Queue
	consumerCount := max(1, queueCfg.ConsumerCount)
	if queueCfg.MaxConsumerCount > 0 {
		consumerCount = min(consumerCount, queueCfg.MaxConsumerCount)
	}
	repo := &queuedUsageBillingRepository{
		direct:         direct,
		db:             sqlDB,
		rdb:            rdb,
		consumerCount:  consumerCount,
		readBatchSize:  max(1, queueCfg.ReadBatchSize),
		pollInterval:   time.Duration(max(1, queueCfg.ReadBlockMilliseconds)) * time.Millisecond,
		commandTimeout: time.Duration(max(1, queueCfg.CommandTimeoutSeconds)) * time.Second,
		maxRetryDelay:  time.Duration(max(1, queueCfg.MaxRetryDelaySeconds)) * time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, consumerCount),
	}
	repo.recoverPendingOverlays()
	repo.start()
	return repo
}

func (r *queuedUsageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("durable usage billing queue db is nil")
	}
	cloned := cloneUsageBillingCommand(cmd)
	cloned.Normalize()
	if cloned.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}
	payload, err := json.Marshal(&cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal usage billing command: %w", err)
	}
	req := usageBillingEnqueueRequest{
		cmd:      cloned,
		payload:  payload,
		resultCh: make(chan usageBillingEnqueueResult, 1),
	}

	r.lifecycle.RLock()
	if r.stopped.Load() {
		r.lifecycle.RUnlock()
		return nil, errors.New("durable usage billing queue is stopped")
	}
	select {
	case r.enqueueCh <- req:
		r.lifecycle.RUnlock()
	case <-ctx.Done():
		r.lifecycle.RUnlock()
		return nil, ctx.Err()
	}

	select {
	case result := <-req.resultCh:
		if result.err != nil {
			return nil, result.err
		}
		switch result.status {
		case usageBillingEnqueueInserted:
			r.reconcilePendingOverlay(&cloned)
			r.wakeConsumers()
			return &service.UsageBillingApplyResult{Applied: true, Deferred: true}, nil
		case usageBillingEnqueuePending:
			// Rebuild the Redis overlay after a Redis restart without double-counting.
			r.reconcilePendingOverlay(&cloned)
			return &service.UsageBillingApplyResult{Applied: false, Deferred: true}, nil
		case usageBillingEnqueueApplied:
			return &service.UsageBillingApplyResult{Applied: false}, nil
		default:
			return nil, service.ErrUsageBillingRequestConflict
		}
	case <-ctx.Done():
		// The batcher uses its own context and will still durably persist this
		// accepted request. A retry is safe because request_id is idempotent.
		return nil, ctx.Err()
	}
}

func cloneUsageBillingCommand(cmd *service.UsageBillingCommand) service.UsageBillingCommand {
	cloned := *cmd
	if cmd.SubscriptionID != nil {
		subscriptionID := *cmd.SubscriptionID
		cloned.SubscriptionID = &subscriptionID
	}
	return cloned
}

func (r *queuedUsageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReserveBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.CaptureBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReleaseBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go r.runEnqueueBatcher(ctx)
	for i := 0; i < r.consumerCount; i++ {
		r.wg.Add(1)
		go r.runConsumer(ctx, i)
	}
}

func (r *queuedUsageBillingRepository) Stop() {
	if r == nil {
		return
	}
	r.lifecycle.Lock()
	if !r.stopped.CompareAndSwap(false, true) {
		r.lifecycle.Unlock()
		return
	}
	r.lifecycle.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *queuedUsageBillingRepository) runEnqueueBatcher(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case first := <-r.enqueueCh:
			batch := r.collectEnqueueBatch(ctx, first)
			r.flushEnqueueBatch(batch)
		case <-ctx.Done():
			for {
				select {
				case first := <-r.enqueueCh:
					batch := r.collectEnqueueBatch(context.Background(), first)
					r.flushEnqueueBatch(batch)
				default:
					return
				}
			}
		}
	}
}

func (r *queuedUsageBillingRepository) collectEnqueueBatch(ctx context.Context, first usageBillingEnqueueRequest) []usageBillingEnqueueRequest {
	batch := make([]usageBillingEnqueueRequest, 0, usageBillingEnqueueBatchMaxSize)
	batch = append(batch, first)
	timer := time.NewTimer(usageBillingEnqueueBatchWindow)
	defer timer.Stop()
	for len(batch) < usageBillingEnqueueBatchMaxSize {
		select {
		case req := <-r.enqueueCh:
			batch = append(batch, req)
		case <-timer.C:
			return batch
		case <-ctx.Done():
			return batch
		}
	}
	return batch
}

func (r *queuedUsageBillingRepository) flushEnqueueBatch(batch []usageBillingEnqueueRequest) {
	if len(batch) == 0 {
		return
	}
	unique := make([]usageBillingEnqueueRequest, 0, len(batch))
	byKey := make(map[string]int, len(batch))
	conflicted := make(map[int]struct{})
	for i, req := range batch {
		key := usageBillingRequestKey(req.cmd.RequestID, req.cmd.APIKeyID)
		idx, exists := byKey[key]
		if !exists {
			byKey[key] = len(unique)
			unique = append(unique, req)
			continue
		}
		if unique[idx].cmd.RequestFingerprint != req.cmd.RequestFingerprint {
			conflicted[i] = struct{}{}
		}
	}

	inputs := make([]usageBillingBatchInput, 0, len(unique))
	for _, req := range unique {
		inputs = append(inputs, usageBillingBatchInput{
			RequestID:          req.cmd.RequestID,
			APIKeyID:           req.cmd.APIKeyID,
			RequestFingerprint: req.cmd.RequestFingerprint,
			Payload:            req.payload,
		})
	}
	payload, err := json.Marshal(inputs)
	results := make(map[string]usageBillingEnqueueResult, len(unique))
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), r.commandTimeout)
		results, err = r.insertEnqueueBatch(ctx, payload)
		cancel()
	}
	if err != nil {
		for _, req := range batch {
			req.resultCh <- usageBillingEnqueueResult{err: err}
		}
		return
	}

	seen := make(map[string]bool, len(unique))
	for i, req := range batch {
		if _, conflict := conflicted[i]; conflict {
			req.resultCh <- usageBillingEnqueueResult{status: usageBillingEnqueueConflict}
			continue
		}
		key := usageBillingRequestKey(req.cmd.RequestID, req.cmd.APIKeyID)
		result, ok := results[key]
		if !ok {
			result.err = errors.New("durable usage billing enqueue result missing")
		}
		if seen[key] && result.status == usageBillingEnqueueInserted {
			result.status = usageBillingEnqueuePending
		}
		seen[key] = true
		req.resultCh <- result
	}
}

func (r *queuedUsageBillingRepository) insertEnqueueBatch(ctx context.Context, payload []byte) (map[string]usageBillingEnqueueResult, error) {
	rows, err := r.db.QueryContext(ctx, usageBillingEnqueueBatchSQL, payload)
	if err != nil {
		return nil, fmt.Errorf("insert durable usage billing batch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	results := make(map[string]usageBillingEnqueueResult)
	for rows.Next() {
		var requestID, status string
		var apiKeyID, jobID int64
		if err := rows.Scan(&requestID, &apiKeyID, &jobID, &status); err != nil {
			return nil, err
		}
		results[usageBillingRequestKey(requestID, apiKeyID)] = usageBillingEnqueueResult{
			status: usageBillingEnqueueStatus(status),
			jobID:  jobID,
		}
	}
	return results, rows.Err()
}
