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
	holds        map[string]*BalanceHoldRecord
	reserveCalls []*BalanceHoldCommand
	captureCalls []*BalanceHoldCommand
	releaseCalls []*BalanceHoldCommand
	bindCalls    int
	statusCalls  []string
	bindErr      error
	captureErr   error
	releaseErr   error
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

func (r *reproVideoBillingRepo) BindBalanceHoldUpstreamRequest(_ context.Context, holdKey, upstreamRequestID string) error {
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
	return nil
}

func (r *reproVideoBillingRepo) GetBalanceHoldByUpstreamRequestID(_ context.Context, upstreamRequestID string) (*BalanceHoldRecord, error) {
	for _, record := range r.holds {
		if record.UpstreamRequestID == upstreamRequestID {
			return record, nil
		}
	}
	return nil, nil
}

func (r *reproVideoBillingRepo) UpdateBalanceHoldStatus(_ context.Context, holdKey, status string, actualAmount *float64) error {
	r.statusCalls = append(r.statusCalls, status)
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

var _ UsageBillingRepository = (*reproVideoBillingRepo)(nil)

// reproGatewayCache 是可注入失败的 GatewayCache 假实现。
type reproGatewayCache struct {
	setErr error
	getErr error
	values map[string]int64
}

func (c *reproGatewayCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	if c.getErr != nil {
		return 0, c.getErr
	}
	return 0, nil
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

	err := svc.BindMultimodalVideoBalanceUpstream(context.Background(), record, "upstream-task-p0-2")
	require.Error(t, err, "绑定失败必须向上暴露")
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Empty(t, record.UpstreamRequestID, "绑定失败不能把内存记录标记为已绑定")
	require.Nil(t, mustLookupHold(t, repo, "upstream-task-p0-2"))
	require.Equal(t, 1, repo.bindCalls)

	// 冻结行存在时绑定必须成功，且能被上游任务 id 反查。
	okRecord := reproExpiredHold(repo, "hold-p0-2-ok", 0.5)
	okRecord.UpstreamRequestID = ""
	require.NoError(t, svc.BindMultimodalVideoBalanceUpstream(context.Background(), okRecord, "upstream-task-p0-2-ok"))
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
	require.Equal(t, int64(99), svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 7,
	))
	require.Zero(t, svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 43, 7,
	), "其它用户不能借用该任务的归属")
	require.Zero(t, svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 8,
	), "其它 API Key 不能借用该任务的归属")

	record.Status = BalanceHoldStatusReleased
	require.Zero(t, svc.ResolveMultimodalVideoBalanceAccountID(
		context.Background(), record.UpstreamRequestID, 42, 7,
	), "已释放的冻结不再提供归属")
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

// TestReproP1_3_UnclassifiedImageSizeRequiresAnyTierPrice 验证 P1-3 修复：
// 请求未给出可分类尺寸时不再猜 2K，只要模型配置了任一图片档位价就放行；
// 完全没有档位价时仍然拒绝。
func TestReproP1_3_UnclassifiedImageSizeRequiresAnyTierPrice(t *testing.T) {
	price := 0.02
	groupID := int64(200)
	group := &Group{
		ID:       groupID,
		Platform: PlatformMultimodal,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"seedance-image"},
			BillingMode: BillingModeImage,
			Intervals: []PricingInterval{{
				TierLabel:       ImageBillingSize1K,
				PerRequestPrice: &price,
			}},
		}},
	}
	group.compileModelPricingIndex()

	billingService := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billingService}
	svc.resolver = NewModelPricingResolver(nil, billingService)
	apiKey := &APIKey{ID: 11, UserID: 42, GroupID: &groupID, Group: group}

	require.NoError(t, svc.ValidateMultimodalImagePricing(context.Background(), MultimodalImagePricingRequest{
		APIKey: apiKey, UserID: 42, GroupID: groupID, Model: "seedance-image", Size: "auto", Count: 1,
	}), "只配置 1K 价时，未指定尺寸的请求不能被 2K 默认值误拒")

	// 完全没有档位价时仍必须拒绝。
	unpriced := &Group{
		ID:       groupID,
		Platform: PlatformMultimodal,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"seedance-image"},
			BillingMode: BillingModeImage,
			Intervals:   []PricingInterval{{TierLabel: ImageBillingSize1K}},
		}},
	}
	unpriced.compileModelPricingIndex()
	unpricedKey := &APIKey{ID: 11, UserID: 42, GroupID: &groupID, Group: unpriced}

	err := svc.ValidateMultimodalImagePricing(context.Background(), MultimodalImagePricingRequest{
		APIKey: unpricedKey, UserID: 42, GroupID: groupID, Model: "seedance-image", Size: "auto", Count: 1,
	})
	require.ErrorIs(t, err, ErrImagePriceNotConfigured)

	// 落账侧同样不能猜档位：请求未给尺寸且上游没有可解析尺寸时必须报错。
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
