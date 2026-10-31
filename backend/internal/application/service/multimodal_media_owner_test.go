//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type multimodalOwnerLookupRepo struct {
	UsageBillingRepository
	VideoBalanceHoldRepository
	record   *BalanceHoldRecord
	err      error
	calls    int
	deadline time.Time
}

func TestForwardCompositeMultimodalVideoRequiresOwnerBindingBeforeResponse(t *testing.T) {
	for _, failBinding := range []bool{false, true} {
		body := []byte(`{"model":"seedance","resolution":"720p"}`)
		svc, c, recorder, upstream, account := multimodalImageForwardFixture(t, body, "application/json",
			strings.NewReader(`{"id":"accepted-task","status":"processing"}`))
		upstream.resp.Header.Set("Content-Type", "application/json")
		cache := &reproGatewayCache{}
		if failBinding {
			cache.setErr = errors.New("cache unavailable")
		}
		svc.cache = cache
		groupID := int64(13)
		key := &APIKey{ID: 7, UserID: 42, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformComposite,
			SubscriptionType: SubscriptionTypeSubscription}}
		c.Set("api_key", key)
		result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointVideosGenerations, "", body, "application/json", nil)
		if failBinding {
			require.ErrorIs(t, err, ErrBalanceHoldFailed)
			require.Nil(t, result)
			require.Empty(t, recorder.Body.String(), "do not acknowledge a task whose owner cannot be recovered")
			continue
		}
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
		owner, err := svc.ResolveMultimodalVideoTaskOwner(context.Background(), key, "accepted-task")
		require.NoError(t, err)
		require.Equal(t, account.ID, owner)
	}
}

func (r *multimodalOwnerLookupRepo) GetBalanceHoldByUpstreamRequestID(ctx context.Context, _ string) (*BalanceHoldRecord, error) {
	r.calls++
	r.deadline, _ = ctx.Deadline()
	return r.record, r.err
}

func TestResolveMultimodalVideoTaskOwnerChecksAllAuthority(t *testing.T) {
	groupID := int64(13)
	key := &APIKey{ID: 7, UserID: 42, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformComposite}}
	for _, tc := range []struct {
		name   string
		mutate func(*BalanceHoldRecord)
		want   int64
	}{
		{name: "pending", want: 99},
		{name: "settled", mutate: func(r *BalanceHoldRecord) { r.Status = BalanceHoldStatusSettled }, want: 99},
		{name: "other user", mutate: func(r *BalanceHoldRecord) { r.UserID++ }},
		{name: "other key", mutate: func(r *BalanceHoldRecord) { r.APIKeyID++ }},
		{name: "other group", mutate: func(r *BalanceHoldRecord) { r.GroupID++ }},
		{name: "released", mutate: func(r *BalanceHoldRecord) { r.Status = BalanceHoldStatusReleased }},
		{name: "unknown status", mutate: func(r *BalanceHoldRecord) { r.Status = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := &BalanceHoldRecord{UserID: 42, APIKeyID: 7, GroupID: groupID, AccountID: 99, Status: BalanceHoldStatusPending}
			if tc.mutate != nil {
				tc.mutate(record)
			}
			repo := &multimodalOwnerLookupRepo{record: record}
			svc := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeStandard}, usageBillingRepo: repo}
			before := time.Now()
			accountID, err := svc.ResolveMultimodalVideoTaskOwner(context.Background(), key, "task-1")
			require.NoError(t, err)
			require.Equal(t, tc.want, accountID)
			require.Equal(t, 1, repo.calls)
			require.WithinDuration(t, before.Add(multimodalVideoOwnerLookupTimeout), repo.deadline, time.Second)
		})
	}
	repo := &multimodalOwnerLookupRepo{err: errors.New("database unavailable")}
	_, err := (&OpenAIGatewayService{usageBillingRepo: repo}).ResolveMultimodalVideoTaskOwner(context.Background(), key, "task-1")
	require.Error(t, err)
}
