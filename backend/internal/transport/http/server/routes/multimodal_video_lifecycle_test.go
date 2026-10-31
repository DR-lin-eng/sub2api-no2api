//go:build unit

package routes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeVideoLifecycleBillingRepo struct {
	service.UsageBillingRepository
	service.VideoBalanceHoldRepository
	record            *service.BalanceHoldRecord
	lookups, captures int
	lookupErr         error
	ownerDeadline     time.Time
}

func (r *compositeVideoLifecycleBillingRepo) GetBalanceHoldByUpstreamRequestID(ctx context.Context, id string) (*service.BalanceHoldRecord, error) {
	r.lookups++
	if r.lookups == 1 {
		r.ownerDeadline, _ = ctx.Deadline()
	}
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if r.record == nil || id != r.record.UpstreamRequestID {
		return nil, nil
	}
	clone := *r.record
	return &clone, nil
}

func (r *compositeVideoLifecycleBillingRepo) CaptureVideoBalance(_ context.Context, input *service.VideoBalanceCapture) (*service.VideoBalanceCaptureResult, error) {
	r.captures++
	if input.HoldKey != r.record.HoldKey {
		return nil, service.ErrBalanceHoldNotFound
	}
	amount := input.UsageLog.ActualCost
	r.record.Status, r.record.ActualAmount = service.BalanceHoldStatusSettled, &amount
	clone := *r.record
	return &service.VideoBalanceCaptureResult{Record: &clone, Billing: &service.UsageBillingApplyResult{Applied: true}}, nil
}

type compositeVideoLifecycleAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r *compositeVideoLifecycleAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if id != r.account.ID {
		return nil, service.ErrNoAvailableAccounts
	}
	clone := r.account
	return &clone, nil
}

type compositeVideoLifecycleKeyRepo struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (r *compositeVideoLifecycleKeyRepo) GetByKeyForAuth(_ context.Context, value string) (*service.APIKey, error) {
	if value != r.key.Key {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *r.key
	return &clone, nil
}

func (r *compositeVideoLifecycleKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}

type compositeVideoLifecycleUpstream struct {
	service.HTTPUpstream
	paths []string
}

func (u *compositeVideoLifecycleUpstream) Do(request *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.paths = append(u.paths, request.URL.Path)
	if accountID != 99 || request.Header.Get("Authorization") != "Bearer upstream-fixture" {
		return nil, errors.New("unexpected upstream authority")
	}
	contentType, body := "application/json", `{"id":"task-owned","status":"completed","seconds":4}`
	if strings.HasSuffix(request.URL.Path, "/content") {
		contentType, body = "video/mp4", "video-content"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func compositeVideoLifecycleRouter(t *testing.T, mutate func(*compositeVideoLifecycleBillingRepo)) (*gin.Engine, *compositeVideoLifecycleBillingRepo, *compositeVideoLifecycleUpstream, *service.APIKey) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 13, Platform: service.PlatformComposite, Status: service.StatusActive, Hydrated: true}
	key := &service.APIKey{ID: 7, UserID: 42, Key: "composite-video-fixture", Status: service.StatusActive,
		GroupID: &group.ID, Group: group, User: &service.User{ID: 42, Status: service.StatusActive, Balance: 0, FrozenBalance: 1}}
	repo := &compositeVideoLifecycleBillingRepo{record: &service.BalanceHoldRecord{
		HoldKey: "hold-owned", UpstreamRequestID: "task-owned", UserID: 42, APIKeyID: 7,
		GroupID: 13, AccountID: 99, Model: "seedance", Resolution: "720p", DurationSeconds: 8,
		HoldAmount: 1, BaseAmount: 1, RateMultiplier: 1, Currency: "USD", Status: service.BalanceHoldStatusPending,
	}}
	if mutate != nil {
		mutate(repo)
	}
	accountRepo := &compositeVideoLifecycleAccountRepo{account: service.Account{ID: 99, Platform: service.PlatformMultimodal,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{13},
		Credentials: map[string]any{"api_key": "upstream-fixture", "base_url": "https://media.example/v1"}}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.MaxBodySize = 1 << 20
	keyService := service.NewAPIKeyService(&compositeVideoLifecycleKeyRepo{key: key}, nil, nil, nil, nil, nil, cfg)
	concurrency := service.NewConcurrencyService(nil)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	upstream := &compositeVideoLifecycleUpstream{}
	gateway := service.NewOpenAIGatewayService(accountRepo, nil, repo, nil, nil, nil, nil, cfg,
		nil, concurrency, service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		nil, nil, nil, nil, nil, nil, nil, nil)
	openAIHandler := handler.NewOpenAIGatewayHandler(gateway, concurrency, billingCache, keyService, nil, nil, nil, nil, cfg)
	router := gin.New()
	RegisterGatewayRoutes(router, &handler.Handlers{OpenAIGateway: openAIHandler, Gateway: &handler.GatewayHandler{},
		APIKey: handler.NewAPIKeyHandler(nil), Announcement: handler.NewAnnouncementHandler(nil),
		AsyncImage: handler.NewAsyncImageHandler(nil, nil), BatchImage: handler.NewBatchImageHandler(nil, nil, nil)},
		servermiddleware.NewAPIKeyAuthMiddleware(keyService, nil, cfg), keyService, nil, nil, nil, nil, cfg)
	return router, repo, upstream, key
}

func TestCompositeMultimodalVideoLifecycleAfterFullBalanceReservation(t *testing.T) {
	router, repo, upstream, key := compositeVideoLifecycleRouter(t, nil)
	for _, path := range []string{"/v1/videos/task-owned", "/videos/task-owned/content"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+key.Key)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}
	require.Equal(t, []string{"/v1/videos/task-owned", "/v1/videos/task-owned/content"}, upstream.paths)
	require.Equal(t, 1, repo.captures, "polling and content delivery must settle only once")
	require.Equal(t, 4, repo.lookups, "one bounded owner read plus one authoritative settlement read per request")
	require.False(t, repo.ownerDeadline.IsZero())
	require.LessOrEqual(t, time.Until(repo.ownerDeadline), 2*time.Second)
}

func TestCompositeMultimodalVideoLookupDoesNotAdmitUnownedTasks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*compositeVideoLifecycleBillingRepo)
		want   int
	}{
		{name: "no hold", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.record = nil }, want: http.StatusForbidden},
		{name: "wrong owner", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.record.UserID++ }, want: http.StatusForbidden},
		{name: "wrong key", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.record.APIKeyID++ }, want: http.StatusForbidden},
		{name: "wrong group", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.record.GroupID++ }, want: http.StatusForbidden},
		{name: "released hold", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.record.Status = service.BalanceHoldStatusReleased }, want: http.StatusForbidden},
		{name: "metadata unavailable", mutate: func(r *compositeVideoLifecycleBillingRepo) { r.lookupErr = errors.New("unavailable") }, want: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, repo, upstream, key := compositeVideoLifecycleRouter(t, tc.mutate)
			request := httptest.NewRequest(http.MethodGet, "/v1/videos/task-owned/content", nil)
			request.Header.Set("Authorization", "Bearer "+key.Key)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
			require.Equal(t, 1, repo.lookups)
			require.Empty(t, upstream.paths)
			require.Zero(t, repo.captures)
		})
	}
}

func TestCompositeMultimodalVideoOwnerLookupAvoidsOtherRequestPaths(t *testing.T) {
	router, repo, _, key := compositeVideoLifecycleRouter(t, nil)
	for _, tc := range []struct {
		method, path, credential string
		want                     int
	}{
		{http.MethodGet, "/v1/videos/task-owned", "", http.StatusUnauthorized},
		{http.MethodPost, "/v1/videos/generations", key.Key, http.StatusForbidden},
		{http.MethodPost, "/v1/chat/completions", key.Key, http.StatusForbidden},
	} {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.credential != "" {
			request.Header.Set("Authorization", "Bearer "+tc.credential)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
	}
	require.Zero(t, repo.lookups, "anonymous requests, generation and chat do not read video metadata")
}
