package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 本文件是 multimodal 视频两段式计费的回归测试。
// 全部使用内存假实现，不访问任何真实上游 API，也不产生真实扣费。

// reproVideoBillingRepo 是 UsageBillingRepository 的内存实现。
// holds 按 holdKey 保存冻结记录，bind 行为与真实 PostgreSQL 实现保持一致：
// 命中 0 行时返回 ErrBalanceHoldNotFound，而不是静默成功。
type reproVideoBillingRepo struct {
	holds          map[string]*BalanceHoldRecord
	reserveCalls   []*BalanceHoldCommand
	captureCalls   []*BalanceHoldCommand
	releaseCalls   []*BalanceHoldCommand
	bindCalls      int
	statusCalls    []string
	bindErr        error
	lookupErr      error
	captureErr     error
	captureApplied *bool
	releaseErr     error
	statusErr      error
}

func newReproVideoBillingRepo() *reproVideoBillingRepo {
	return &reproVideoBillingRepo{holds: map[string]*BalanceHoldRecord{}}
}

func (r *reproVideoBillingRepo) Apply(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	return &UsageBillingApplyResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) ReserveBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) CaptureBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) ReleaseBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) ReserveBalance(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	r.reserveCalls = append(r.reserveCalls, cmd)
	return &BalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) CaptureBalance(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	r.captureCalls = append(r.captureCalls, cmd)
	if r.captureErr != nil {
		return nil, r.captureErr
	}
	if r.captureApplied != nil && !*r.captureApplied {
		// 真实仓储按 request id 幂等：同一指纹重复提交时返回 Applied=false。
		return &BalanceHoldResult{Applied: false}, nil
	}
	return &BalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) ReleaseBalance(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	r.releaseCalls = append(r.releaseCalls, cmd)
	if r.releaseErr != nil {
		return nil, r.releaseErr
	}
	return &BalanceHoldResult{Applied: true}, nil
}

func (r *reproVideoBillingRepo) CreateBalanceHold(_ context.Context, record *BalanceHoldRecord) error {
	if record == nil {
		return nil
	}
	copied := *record
	r.holds[copied.HoldKey] = &copied
	return nil
}

func (r *reproVideoBillingRepo) BindBalanceHoldUpstreamRequest(_ context.Context, holdKey, upstreamRequestID string, accountID int64) error {
	r.bindCalls++
	if r.bindErr != nil {
		return r.bindErr
	}
	record := r.holds[holdKey]
	if record == nil {
		// 真实 SQL 是 UPDATE ... WHERE hold_key = $1，命中 0 行必须报错，
		// 否则调用方会把「库里没有这条冻结」误认为绑定成功。
		return ErrBalanceHoldNotFound
	}
	record.UpstreamRequestID = upstreamRequestID
	if accountID > 0 {
		record.AccountID = accountID
	}
	return nil
}

func (r *reproVideoBillingRepo) GetBalanceHoldByUpstreamRequestID(_ context.Context, upstreamRequestID string) (*BalanceHoldRecord, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	for _, record := range r.holds {
		if record.UpstreamRequestID == upstreamRequestID {
			return record, nil
		}
	}
	return nil, nil
}

func (r *reproVideoBillingRepo) UpdateBalanceHoldStatus(_ context.Context, holdKey, status string, actualAmount *float64) error {
	r.statusCalls = append(r.statusCalls, status)
	if r.statusErr != nil {
		return r.statusErr
	}
	record := r.holds[holdKey]
	if record == nil {
		return nil
	}
	if record.Status != BalanceHoldStatusPending {
		return nil
	}
	record.Status = status
	if actualAmount != nil {
		value := *actualAmount
		record.ActualAmount = &value
	}
	return nil
}

func (r *reproVideoBillingRepo) ListExpiredBalanceHolds(_ context.Context, now time.Time, limit int) ([]*BalanceHoldRecord, error) {
	out := make([]*BalanceHoldRecord, 0, len(r.holds))
	for _, record := range r.holds {
		if record.Status != BalanceHoldStatusPending {
			continue
		}
		if record.ExpiresAt.After(now) {
			continue
		}
		out = append(out, record)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *reproVideoBillingRepo) ReserveVideoBalance(ctx context.Context, record *BalanceHoldRecord) (*BalanceHoldRecord, error) {
	if err := r.CreateBalanceHold(ctx, record); err != nil {
		return nil, err
	}
	return r.holds[record.HoldKey], nil
}

func (r *reproVideoBillingRepo) CaptureVideoBalance(ctx context.Context, input *VideoBalanceCapture) (*VideoBalanceCaptureResult, error) {
	record := r.holds[input.HoldKey]
	if record == nil {
		return nil, ErrBalanceHoldNotFound
	}
	if record.Status != BalanceHoldStatusPending {
		return &VideoBalanceCaptureResult{Record: record}, nil
	}
	cmd := &BalanceHoldCommand{RequestID: BalanceCaptureRequestID(record.HoldKey), APIKeyID: record.APIKeyID,
		UserID: record.UserID, HoldKey: record.HoldKey, HoldAmount: record.HoldAmount, ActualAmount: input.UsageLog.ActualCost}
	applied, err := r.CaptureBalance(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err := r.UpdateBalanceHoldStatus(ctx, record.HoldKey, BalanceHoldStatusSettled, &cmd.ActualAmount); err != nil {
		return nil, err
	}
	return &VideoBalanceCaptureResult{Record: record, Billing: &UsageBillingApplyResult{Applied: applied.Applied}}, nil
}

func (r *reproVideoBillingRepo) ReleaseVideoBalance(ctx context.Context, record *BalanceHoldRecord) (*BalanceHoldRecord, error) {
	current := r.holds[record.HoldKey]
	if current == nil {
		return nil, ErrBalanceHoldNotFound
	}
	if current.Status != BalanceHoldStatusPending {
		return current, nil
	}
	_, err := r.ReleaseBalance(ctx, &BalanceHoldCommand{RequestID: BalanceReleaseRequestID(record.HoldKey),
		APIKeyID: record.APIKeyID, UserID: record.UserID, HoldKey: record.HoldKey, HoldAmount: record.HoldAmount})
	if err != nil {
		return nil, err
	}
	if err := r.UpdateBalanceHoldStatus(ctx, record.HoldKey, BalanceHoldStatusReleased, nil); err != nil {
		return nil, err
	}
	return current, nil
}

var _ VideoBalanceHoldRepository = (*reproVideoBillingRepo)(nil)

var _ UsageBillingRepository = (*reproVideoBillingRepo)(nil)

// reproGatewayCache 是可注入失败的 GatewayCache 假实现。
type reproGatewayCache struct {
	setErr error
	getErr error
	values map[string]int64
}

func (c *reproGatewayCache) GetSessionAccountID(_ context.Context, _ int64, key string) (int64, error) {
	if c.getErr != nil {
		return 0, c.getErr
	}
	return c.values[key], nil
}

func (c *reproGatewayCache) SetSessionAccountID(_ context.Context, _ int64, key string, accountID int64, _ time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	if c.values == nil {
		c.values = map[string]int64{}
	}
	c.values[key] = accountID
	return nil
}

func (c *reproGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *reproGatewayCache) DeleteSessionAccountID(context.Context, int64, string) error { return nil }

var _ GatewayCache = (*reproGatewayCache)(nil)

func reproExpiredHold(repo *reproVideoBillingRepo, holdKey string, amount float64) *BalanceHoldRecord {
	now := time.Now()
	record := &BalanceHoldRecord{
		HoldKey:           holdKey,
		UpstreamRequestID: "upstream-" + holdKey,
		UserID:            42,
		APIKeyID:          7,
		GroupID:           3,
		AccountID:         99,
		Model:             "grok-video-repro",
		Resolution:        VideoBillingResolution720P,
		DurationSeconds:   8,
		HoldAmount:        amount,
		Currency:          videoBalanceHoldCurrency,
		Status:            BalanceHoldStatusPending,
		ExpiresAt:         now.Add(-time.Minute),
		CreatedAt:         now.Add(-time.Hour),
		UpdatedAt:         now.Add(-time.Hour),
	}
	repo.holds[record.HoldKey] = record
	return record
}

func reproGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 7, UserID: 42})
	return c
}

// TestReproP0_1_VideoContentSettlesBeforeDelivery 验证 P0-1 修复：
// 只调 /v1/videos/{id}/content 取片、不轮询 status 时，内容交付前也必须结算；
// 结算后 sweeper 不能再把冻结金额退还给用户。
func TestReproP0_1_VideoContentSettlesBeforeDelivery(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p0-1", 0.5)

	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproGinContext(), nil, record.UpstreamRequestID,
	))
	require.Len(t, repo.captureCalls, 1, "取片必须触发 CaptureBalance")
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status)
	require.NotNil(t, repo.holds[record.HoldKey].ActualAmount)
	require.InDelta(t, 0.5, *repo.holds[record.HoldKey].ActualAmount, 1e-9)

	// 重复取片必须幂等：已结算的记录不再扣费。
	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproGinContext(), nil, record.UpstreamRequestID,
	))
	require.Len(t, repo.captureCalls, 1, "重复取片不能重复扣费")

	svc.sweepExpiredVideoBalanceHolds(context.Background())
	require.Empty(t, repo.releaseCalls, "已结算的冻结不允许被 sweeper 释放")
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status)
}

// TestReproP0_2_BindFailureSurfacesError 验证 P0-2 修复：
// 冻结行不存在（UPDATE 命中 0 行）时绑定必须失败并返回错误，
// 不允许把内存记录标记成已绑定后继续交付。
func TestReproP0_2_BindFailureSurfacesError(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p0-2", 0.5)
	record.UpstreamRequestID = ""
	// 模拟真实 SQL 命中 0 行：数据库里没有这条 hold_key。
	delete(repo.holds, record.HoldKey)

	err := svc.BindMultimodalVideoBalanceUpstream(context.Background(), record, 0, "upstream-task-p0-2")
	require.Error(t, err, "绑定失败必须向上暴露")
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Empty(t, record.UpstreamRequestID, "绑定失败不能把内存记录标记为已绑定")
	require.Nil(t, mustLookupHold(t, repo, "upstream-task-p0-2"))
	require.Equal(t, 1, repo.bindCalls)

	// 冻结行存在时绑定必须成功，且能被上游任务 id 反查。
	okRecord := reproExpiredHold(repo, "hold-p0-2-ok", 0.5)
	okRecord.UpstreamRequestID = ""
	require.NoError(t, svc.BindMultimodalVideoBalanceUpstream(context.Background(), okRecord, 0, "upstream-task-p0-2-ok"))
	require.Equal(t, "upstream-task-p0-2-ok", okRecord.UpstreamRequestID)
	require.NotNil(t, mustLookupHold(t, repo, "upstream-task-p0-2-ok"))
}

func mustLookupHold(t *testing.T, repo *reproVideoBillingRepo, upstreamRequestID string) *BalanceHoldRecord {
	t.Helper()
	record, err := repo.GetBalanceHoldByUpstreamRequestID(context.Background(), upstreamRequestID)
	require.NoError(t, err)
	return record
}

// TestReproP0_3_PollingFallsBackToBalanceHoldOwner 验证 P0-3 修复：
// 创建期归属缓存写入失败时必须返回错误（调用方释放冻结）；
// 归属缓存丢失时轮询仍可用冻结记录里的账号恢复归属，不再直接 404。
func TestReproP0_3_PollingFallsBackToBalanceHoldOwner(t *testing.T) {
	repo := newReproVideoBillingRepo()
	cache := &reproGatewayCache{setErr: errors.New("redis unavailable")}
	svc := &OpenAIGatewayService{usageBillingRepo: repo, cache: cache}
	record := reproExpiredHold(repo, "hold-p0-3", 0.5)

	err := svc.bindMultimodalVideoRequestOwner(
		context.Background(), reproGinContext(), &Account{ID: record.AccountID}, record, record.UpstreamRequestID,
	)
	require.Error(t, err, "归属写入失败必须在交付前暴露")
	require.ErrorIs(t, err, ErrBalanceHoldFailed)

	// 归属缓存不可用时，冻结记录仍是权威来源。
	recovered, err := svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 7,
	)
	require.NoError(t, err)
	require.Equal(t, int64(99), recovered)

	otherUser, err := svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 43, 7,
	)
	require.NoError(t, err)
	require.Zero(t, otherUser, "其它用户不能借用该任务的归属")

	otherKey, err := svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 8,
	)
	require.NoError(t, err)
	require.Zero(t, otherKey, "其它 API Key 不能借用该任务的归属")

	record.Status = BalanceHoldStatusReleased
	released, err := svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 7,
	)
	require.NoError(t, err)
	require.Zero(t, released, "已释放的冻结不再提供归属")

	// 查询失败必须与「确认无记录」区分：前者返回错误，不能被当成任务不存在。
	repo.lookupErr = errors.New("database unavailable")
	_, lookupErr := svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 7,
	)
	require.Error(t, lookupErr, "查询失败必须向上暴露")
}

// TestReproP0_4_FailoverPersistsFinalOwnerAccount 验证阶段 A 修复：
// 首次选号账号 A 失败、账号 B 成功创建任务时，冻结记录归属必须改写为 B；
// 归属缓存有效、缺失或过期（等价于缓存不可用）时轮询都必须解析为 B。
func TestReproP0_4_FailoverPersistsFinalOwnerAccount(t *testing.T) {
	repo := newReproVideoBillingRepo()
	cache := &reproGatewayCache{}
	svc := &OpenAIGatewayService{usageBillingRepo: repo, cache: cache}
	record := reproExpiredHold(repo, "hold-p0-4", 0.5)
	// 模拟创建阶段在首次选号账号 A 上建立冻结。
	record.AccountID = 101
	repo.holds[record.HoldKey].AccountID = 101

	// 账号 A 提交失败后由账号 B 成功创建任务：绑定必须同时持久化账号 B。
	require.NoError(t, svc.BindMultimodalVideoBalanceUpstream(
		context.Background(), record, 202, "upstream-task-p0-4",
	))
	require.Equal(t, int64(202), record.AccountID)
	require.Equal(t, int64(202), repo.holds[record.HoldKey].AccountID, "冻结记录必须持久化最终成功账号")

	ctx := context.Background()
	c := reproGinContext()
	require.NoError(t, svc.bindMultimodalVideoRequestOwner(ctx, c, &Account{ID: 202}, record, record.UpstreamRequestID))

	// 缓存有效与缓存缺失（缓存过期等价于查不到 key）都必须解析为账号 B。
	cachedAccountID, err := svc.ResolveMultimodalMediaVideoRequestAccount(
		ctx, nil, record.UpstreamRequestID, 42, 7,
	)
	require.NoError(t, err)
	require.Equal(t, int64(202), cachedAccountID, "缓存有效时解析为最终账号 B")

	cache.values = nil
	missingAccountID, err := svc.ResolveMultimodalMediaVideoRequestAccount(
		ctx, nil, record.UpstreamRequestID, 42, 7,
	)
	require.NoError(t, err)
	require.Zero(t, missingAccountID, "缓存缺失时缓存侧查不到归属")

	recoveredAccountID, err := svc.ResolveMultimodalVideoBalanceAccountID(
		ctx, record.UpstreamRequestID, 42, 7,
	)
	require.NoError(t, err)
	require.Equal(t, int64(202), recoveredAccountID, "缓存缺失时必须从冻结记录恢复为账号 B")

	// accountID <= 0 不得改写已有归属（同账号重试路径）。
	require.NoError(t, svc.BindMultimodalVideoBalanceUpstream(
		ctx, record, 0, "upstream-task-p0-4-retry",
	))
	require.Equal(t, int64(202), repo.holds[record.HoldKey].AccountID)
}

// reproHoldModeContext 构造一个明确属于冻结计费模式的请求上下文（非订阅制分组）。
func reproHoldModeContext() *gin.Context {
	c := reproGinContext()
	c.Set("api_key", &APIKey{ID: 7, UserID: 42})
	return c
}

// reproSubscriptionContext 构造一个订阅制分组的请求上下文：该模式不产生冻结。
func reproSubscriptionContext() *gin.Context {
	c := reproGinContext()
	c.Set("api_key", &APIKey{ID: 7, UserID: 42, Group: &Group{SubscriptionType: SubscriptionTypeSubscription}})
	return c
}

// TestReproP0_5_HoldLookupFailureFailsClosed 验证阶段 B 修复：
// 冻结查询失败必须与「确认无记录」区分，状态查询和取片都不得当成正常无 hold 放行；
// 只有能从认证上下文明确判定不需要冻结（订阅制）时才允许继续。
func TestReproP0_5_HoldLookupFailureFailsClosed(t *testing.T) {
	repo := newReproVideoBillingRepo()
	repo.lookupErr = errors.New("database unavailable")
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p0-5", 0.5)
	body := []byte(`{"id":"upstream-hold-p0-5","status":"completed","seconds":8}`)

	// 状态查询：查询失败返回可重试错误，响应原样返回、不追加扣费字段。
	rendered, err := svc.applyMultimodalVideoBilling(
		context.Background(), reproGinContext(), nil,
		MultimodalMediaEndpointVideoStatus, record.UpstreamRequestID, nil, body,
	)
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Equal(t, body, rendered)
	require.Empty(t, repo.captureCalls)

	// 取片：冻结计费模式下查询失败不得放行内容，也不得猜测定价结算。
	require.ErrorIs(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproHoldModeContext(), nil, record.UpstreamRequestID,
	), ErrBalanceHoldFailed)
	require.Empty(t, repo.captureCalls)

	// 订阅制分组不需要冻结：查询失败不阻塞交付。
	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproSubscriptionContext(), nil, record.UpstreamRequestID,
	))
	require.Empty(t, repo.captureCalls)
}

// TestReproP0_6_MissingHoldRecordFailsClosedForContentDelivery 验证阶段 B 修复：
// 冻结计费模式下确认「没有冻结记录」不是合法的无 hold 路径，不得免费交付内容；
// 订阅制分组等明确不需要冻结的请求仍可正常交付。
func TestReproP0_6_MissingHoldRecordFailsClosedForContentDelivery(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}

	err := svc.settleMultimodalVideoContentBalance(
		context.Background(), reproHoldModeContext(), nil, "upstream-task-unknown",
	)
	require.ErrorIs(t, err, ErrBalanceHoldFailed, "冻结计费模式下缺少冻结记录不得免费交付")
	require.Empty(t, repo.captureCalls)

	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproSubscriptionContext(), nil, "upstream-task-unknown",
	), "订阅制分组没有冻结记录属于合法无 hold 路径")
	require.Empty(t, repo.captureCalls)
}

// TestReproP1_1_CaptureFailureSurfacesErrorAndKeepsPending 验证 P1-1 修复：
// 落账失败时必须返回错误、响应不回显冻结金额、记录保持 pending 等待重试；
// 重试成功后正常结算，不会把已完成的视频变成免费。
func TestReproP1_1_CaptureFailureSurfacesErrorAndKeepsPending(t *testing.T) {
	repo := newReproVideoBillingRepo()
	repo.captureErr = errors.New("database unavailable")
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p1-1", 0.5)
	body := []byte(`{"id":"upstream-hold-p1-1","status":"completed","seconds":8}`)

	rendered, err := svc.applyMultimodalVideoBilling(
		context.Background(), reproGinContext(), nil,
		MultimodalMediaEndpointVideoStatus, record.UpstreamRequestID, nil, body,
	)
	require.Error(t, err, "落账失败必须返回错误")
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.NotContains(t, string(rendered), "cost", "落账失败不能回显已扣费")
	require.Equal(t, BalanceHoldStatusPending, repo.holds[record.HoldKey].Status, "落账失败后记录保持 pending 以便重试")
	require.Len(t, repo.captureCalls, 1)

	// 已绑定上游任务的冻结在宽限期内不能被 sweeper 退款。
	svc.sweepExpiredVideoBalanceHolds(context.Background())
	require.Empty(t, repo.releaseCalls)
	require.Equal(t, BalanceHoldStatusPending, repo.holds[record.HoldKey].Status)

	// 下一次轮询重试成功，正常结算并回显金额。
	repo.captureErr = nil
	retried, retryErr := svc.applyMultimodalVideoBilling(
		context.Background(), reproGinContext(), nil,
		MultimodalMediaEndpointVideoStatus, record.UpstreamRequestID, nil, body,
	)
	require.NoError(t, retryErr)
	require.Contains(t, string(retried), `"cost":0.5`)
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status)
	require.Len(t, repo.captureCalls, 2)
}

// TestReproP1_2_RequestImageTierIsLockedAgainstUpstreamOutput 验证 P1-2 修复：
// 请求给出了可分类尺寸时，计费档位锁定为请求档位，不被上游输出尺寸改档。
func TestReproP1_2_RequestImageTierIsLockedAgainstUpstreamOutput(t *testing.T) {
	requestInfo := GrokMediaRequestInfo{Size: "1024x1024"}
	responseBody := []byte(`{"data":[{"url":"https://example.test/a.png","size":"3840x2160"}]}`)

	meta := multimodalMediaUsageFromResponse(
		MultimodalMediaEndpointImagesGenerations, requestInfo, nil, responseBody,
	)
	require.Equal(t, ImageBillingSize1K, meta.ImageSize, "multimodal 图片按请求档位计费")
	require.True(t, meta.ImageBillingSizeLocked)
	require.Equal(t, []string{"3840x2160"}, meta.ImageOutputSizes)

	grokMeta := grokMediaUsageFromResponse(GrokMediaEndpointImagesGenerations, requestInfo, responseBody)
	require.Equal(t, ImageBillingSize1K, grokMeta.ImageSize, "grok 图片按请求档位计费")
	require.True(t, grokMeta.ImageBillingSizeLocked)

	// 未指定尺寸时不锁定档位，落账按上游实际输出解析。
	autoMeta := multimodalMediaUsageFromResponse(
		MultimodalMediaEndpointImagesGenerations, GrokMediaRequestInfo{Size: "auto"}, nil, responseBody,
	)
	require.Empty(t, autoMeta.ImageSize)
	require.False(t, autoMeta.ImageBillingSizeLocked)

	// 若缺少锁定位，落账时 ApplyOpenAIImageBillingResolution 会把 1K 改写成 4K。
	unlocked := &OpenAIForwardResult{
		ImageCount:       1,
		ImageSize:        ImageBillingSize1K,
		ImageInputSize:   "1024x1024",
		ImageOutputSizes: []string{"3840x2160"},
	}
	ApplyOpenAIImageBillingResolution(unlocked)
	require.Equal(t, ImageBillingSize4K, unlocked.ImageSize, "证明锁定位是必需的")
}

// TestReproP1_3_UnclassifiedImageSizeRequiresAllTierPrices 验证 P1-3 修复：
// 请求未给出可分类尺寸时不能猜 2K，实际结算档位只能由上游输出决定，
// 因此只有全部档位都配价才放行；只配部分档位或完全没有档位价时都必须拒绝。
func TestReproP1_3_UnclassifiedImageSizeRequiresAllTierPrices(t *testing.T) {
	price := 0.02
	groupID := int64(200)
	buildGroup := func(intervals []PricingInterval) *Group {
		group := &Group{
			ID:       groupID,
			Platform: PlatformMultimodal,
			ModelPricing: []ChannelModelPricing{{
				Models:      []string{"seedance-image"},
				BillingMode: BillingModeImage,
				Intervals:   intervals,
			}},
		}
		group.compileModelPricingIndex()
		return group
	}

	billingService := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billingService}
	svc.resolver = NewModelPricingResolver(nil, billingService)
	request := func(group *Group) MultimodalImagePricingRequest {
		return MultimodalImagePricingRequest{
			APIKey:  &APIKey{ID: 11, UserID: 42, GroupID: &groupID, Group: group},
			UserID:  42,
			GroupID: groupID,
			Model:   "seedance-image",
			Size:    "auto",
			Count:   1,
		}
	}

	// 只配置 1K 价：上游可能返回 2K/4K，落账会因实际档位缺价而无法结算，必须在提交前拒绝。
	partial := buildGroup([]PricingInterval{{
		TierLabel:       ImageBillingSize1K,
		PerRequestPrice: &price,
	}})
	require.ErrorIs(t, svc.ValidateMultimodalImagePricing(context.Background(), request(partial)),
		ErrImagePriceNotConfigured, "只配置 1K 价时，未指定尺寸的请求不能放行")

	// 全部档位有价：任意输出档位都可结算，才允许提交。
	allTiers := buildGroup([]PricingInterval{
		{TierLabel: ImageBillingSize1K, PerRequestPrice: &price},
		{TierLabel: ImageBillingSize2K, PerRequestPrice: &price},
		{TierLabel: ImageBillingSize4K, PerRequestPrice: &price},
	})
	require.NoError(t, svc.ValidateMultimodalImagePricing(context.Background(), request(allTiers)),
		"全部档位有价时，未指定尺寸的请求必须放行")

	// 完全没有档位价时仍必须拒绝。
	unpriced := buildGroup([]PricingInterval{{TierLabel: ImageBillingSize1K}})
	require.ErrorIs(t, svc.ValidateMultimodalImagePricing(context.Background(), request(unpriced)),
		ErrImagePriceNotConfigured)

	// 落账侧同样不能猜档位：请求未给尺寸且上游没有可解析尺寸时必须报错。
	apiKey := &APIKey{ID: 11, UserID: 42, GroupID: &groupID, Group: partial}
	_, costErr := svc.calculateOpenAIImageCost(context.Background(), "seedance-image", apiKey, &OpenAIForwardResult{
		ImageCount: 1,
	}, 1)
	require.Error(t, costErr, "无法分类尺寸时不能按 2K 默认档位计费")

	lockedCost, lockedErr := svc.calculateOpenAIImageCost(context.Background(), "seedance-image", apiKey, &OpenAIForwardResult{
		ImageCount:             1,
		ImageSize:              ImageBillingSize1K,
		ImageBillingSizeLocked: true,
	}, 1)
	require.NoError(t, lockedErr)
	require.NotNil(t, lockedCost)
	require.InDelta(t, 0.02, lockedCost.TotalCost, 1e-9)
}

// TestReproP1_5_DeliveryRequiresActualTierPrice 验证 P1-3 的交付顺序修复：
// 响应写回客户端前必须按上游实际输出确定计费档位并确认该档位有价，
// 否则客户端会先拿到无法结算的图片，缺价只在落账阶段写日志。
func TestReproP1_5_DeliveryRequiresActualTierPrice(t *testing.T) {
	price1K := 0.01
	price2K := 0.02
	groupID := int64(201)
	group := &Group{
		ID:       groupID,
		Platform: PlatformMultimodal,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"seedance-image"},
			BillingMode: BillingModeImage,
			Intervals: []PricingInterval{
				{TierLabel: ImageBillingSize1K, PerRequestPrice: &price1K},
				{TierLabel: ImageBillingSize2K, PerRequestPrice: &price2K},
			},
		}},
	}
	group.compileModelPricingIndex()

	billingService := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billingService}
	svc.resolver = NewModelPricingResolver(nil, billingService)
	apiKey := &APIKey{ID: 11, UserID: 42, GroupID: &groupID, Group: group}

	// auto 请求未锁定档位：上游返回 4K 时结算档位是 4K，而 4K 未配置价格。
	require.ErrorIs(t, svc.validateMultimodalImageSettlement(context.Background(), "seedance-image", apiKey,
		multimodalMediaUsageMetadata{
			ImageCount:       1,
			ImageInputSize:   "auto",
			ImageOutputSizes: []string{"3840x2160"},
		}), ErrImagePriceNotConfigured, "实际输出档位缺价时不能在交付前放行")

	// 同一请求只要上游输出 1K，档位有价即可交付。
	require.NoError(t, svc.validateMultimodalImageSettlement(context.Background(), "seedance-image", apiKey,
		multimodalMediaUsageMetadata{
			ImageCount:       1,
			ImageInputSize:   "auto",
			ImageOutputSizes: []string{"1024x1024"},
		}))

	// 多个输出尺寸按最高档位计费，最高档缺价同样必须拒绝。
	require.ErrorIs(t, svc.validateMultimodalImageSettlement(context.Background(), "seedance-image", apiKey,
		multimodalMediaUsageMetadata{
			ImageCount:       2,
			ImageInputSize:   "auto",
			ImageOutputSizes: []string{"1024x1024", "4096x4096"},
		}), ErrImagePriceNotConfigured, "多输出尺寸时最高档缺价必须拒绝")

	// 请求已给出可分类尺寸时档位锁定为请求档位，不受上游输出尺寸影响。
	require.NoError(t, svc.validateMultimodalImageSettlement(context.Background(), "seedance-image", apiKey,
		multimodalMediaUsageMetadata{
			ImageCount:             1,
			ImageSize:              ImageBillingSize1K,
			ImageInputSize:         "1024x1024",
			ImageOutputSizes:       []string{"3840x2160"},
			ImageBillingSizeLocked: true,
		}))

	// 简单模式只记录用量、不扣费，不因缺价阻断交付。
	simpleSvc := &OpenAIGatewayService{
		billingService: billingService,
		cfg:            &config.Config{RunMode: config.RunModeSimple},
	}
	simpleSvc.resolver = NewModelPricingResolver(nil, billingService)
	require.NoError(t, simpleSvc.validateMultimodalImageSettlement(context.Background(), "seedance-image", apiKey,
		multimodalMediaUsageMetadata{
			ImageCount:       1,
			ImageInputSize:   "auto",
			ImageOutputSizes: []string{"3840x2160"},
		}))
}

// TestReproP1_4_UnclassifiableVideoResolutionIsRejected 验证 P1-4 修复：
// 无法映射到计费档位的分辨率在提交上游前被拒绝，不再静默按 480p 计费。
func TestReproP1_4_UnclassifiableVideoResolutionIsRejected(t *testing.T) {
	tier, ok := ClassifyVideoBillingResolution("1440p")
	require.False(t, ok)
	require.Empty(t, tier)

	require.ErrorIs(t, ValidateMultimodalVideoResolution("4k"), ErrVideoPriceNotConfigured)
	require.ErrorIs(t, ValidateMultimodalVideoResolution("1440p"), ErrVideoPriceNotConfigured)
	require.NoError(t, ValidateMultimodalVideoResolution(""), "未指定分辨率保持放行")
	require.NoError(t, ValidateMultimodalVideoResolution("720p"))
	require.NoError(t, ValidateMultimodalVideoResolution("1080P"))

	info := ParseGrokMediaRequest(
		"application/json",
		[]byte(`{"model":"grok-imagine-video","prompt":"p","resolution":"1440p"}`),
	)
	require.Equal(t, "1440p", info.ResolutionRaw, "原始分辨率文本必须保留用于拦截")
	require.ErrorIs(t, ValidateMultimodalVideoResolution(info.ResolutionRaw), ErrVideoPriceNotConfigured)
}

// TestReproP1_6_SettlementStatusFailureConvergesOnRetry 验证阶段 D 修复：
// 资金已落账但状态写失败时必须返回可重试错误；重试命中 capture 幂等重放
// （Applied=false）时仍要把记录推进到 settled，否则 sweeper 会把已扣费任务退款。
func TestReproP1_6_SettlementStatusFailureConvergesOnRetry(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p1-6", 0.5)
	repo.statusErr = errors.New("database unavailable")

	_, err := svc.SettleMultimodalVideoBalance(context.Background(), reproGinContext(), nil, record, 8)
	require.ErrorIs(t, err, ErrBalanceHoldFailed, "状态写失败必须让调用方重试")
	require.Equal(t, BalanceHoldStatusPending, repo.holds[record.HoldKey].Status)
	require.Len(t, repo.captureCalls, 1)

	// 重试：capture 已按同一指纹落账，仓储返回 Applied=false。
	notApplied := false
	repo.captureApplied = &notApplied
	repo.statusErr = nil
	settled, retryErr := svc.SettleMultimodalVideoBalance(context.Background(), reproGinContext(), nil, record, 8)
	require.NoError(t, retryErr)
	require.NotNil(t, settled)
	require.InDelta(t, 0.5, *settled, 1e-9)
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status, "幂等重放也必须收敛状态")
	require.NotNil(t, repo.holds[record.HoldKey].ActualAmount)
	require.Len(t, repo.captureCalls, 2)

	// 已结算记录不再触发 capture，也不能被 sweeper 释放。
	again, err := svc.SettleMultimodalVideoBalance(context.Background(), reproGinContext(), nil, record, 8)
	require.NoError(t, err)
	require.NotNil(t, again)
	require.InDelta(t, 0.5, *again, 1e-9)
	require.Len(t, repo.captureCalls, 2)

	svc.sweepExpiredVideoBalanceHolds(context.Background())
	require.Empty(t, repo.releaseCalls, "已收敛为 settled 的冻结不允许被 sweeper 释放")
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status)
}

// TestReproP1_7_ZeroHoldSettlementIsCapped 验证阶段 D 修复：
// 冻结额为 0 时仓储会拒绝正结算额（ErrBalanceHoldCostExceedsHold），
// 结算侧必须按冻结额封顶为 0 并正常收敛，不能让记录永久停留在 pending。
func TestReproP1_7_ZeroHoldSettlementIsCapped(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p1-7", 0)

	settled, err := svc.SettleMultimodalVideoBalance(context.Background(), reproGinContext(), nil, record, 8)
	require.NoError(t, err)
	require.NotNil(t, settled)
	require.Zero(t, *settled, "没有任何冻结资金时结算额只能是 0")
	require.Len(t, repo.captureCalls, 1)
	require.Zero(t, repo.captureCalls[0].ActualAmount, "不能把正结算额送进必然被拒绝的 capture")
	require.Equal(t, BalanceHoldStatusSettled, repo.holds[record.HoldKey].Status)
}

// TestReproP1_8_ReleasedHoldRejectsContentDelivery 验证阶段 D 修复：
// 已释放的冻结等于已经退款，内容不能再免费交付；订阅制等无需冻结的请求仍可交付，
// 已结算的冻结重复取片也仍然放行。
func TestReproP1_8_ReleasedHoldRejectsContentDelivery(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p1-8", 0.5)
	record.Status = BalanceHoldStatusReleased

	require.ErrorIs(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproHoldModeContext(), nil, record.UpstreamRequestID,
	), ErrBalanceHoldFailed, "已释放的冻结不得继续交付内容")
	require.Empty(t, repo.captureCalls)

	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproSubscriptionContext(), nil, record.UpstreamRequestID,
	), "订阅制分组本来就没有冻结，已释放状态不影响交付")
	require.Empty(t, repo.captureCalls)

	record.Status = BalanceHoldStatusSettled
	require.NoError(t, svc.settleMultimodalVideoContentBalance(
		context.Background(), reproHoldModeContext(), nil, record.UpstreamRequestID,
	), "已结算的冻结重复取片仍然放行")
	require.Empty(t, repo.captureCalls)
}

// TestReproP1_9_ReleaseStatusFailureSurfacesRetryableError 验证阶段 D 修复：
// 资金已归还但状态写失败时不能静默成功，必须让调用方重试；
// 重试使用同一 release request id，资金不会重复归还，最终收敛为 released。
func TestReproP1_9_ReleaseStatusFailureSurfacesRetryableError(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	record := reproExpiredHold(repo, "hold-p1-9", 0.5)
	repo.statusErr = errors.New("database unavailable")

	require.ErrorIs(t, svc.ReleaseMultimodalVideoBalance(context.Background(), record), ErrBalanceHoldFailed)
	require.Equal(t, BalanceHoldStatusPending, repo.holds[record.HoldKey].Status)
	require.Len(t, repo.releaseCalls, 1)

	repo.statusErr = nil
	require.NoError(t, svc.ReleaseMultimodalVideoBalance(context.Background(), record))
	require.Equal(t, BalanceHoldStatusReleased, repo.holds[record.HoldKey].Status)
	require.Len(t, repo.releaseCalls, 2)
	require.Equal(t, repo.releaseCalls[0].RequestID, repo.releaseCalls[1].RequestID,
		"重试必须沿用同一 release request id，由仓储幂等保证只退款一次")
}
