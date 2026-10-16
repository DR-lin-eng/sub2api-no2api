//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

type refundSubscriptionRepo struct {
	userSubRepoNoop
	sub          UserSubscription
	restoreCalls int
}

func (r *refundSubscriptionRepo) GetByID(context.Context, int64) (*UserSubscription, error) {
	if r.sub.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	copy := r.sub
	return &copy, nil
}

func (r *refundSubscriptionRepo) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	copy := r.sub
	return &copy, nil
}

func (r *refundSubscriptionRepo) ExtendExpiry(_ context.Context, _ int64, expires time.Time) error {
	r.sub.ExpiresAt = expires
	return nil
}

func (r *refundSubscriptionRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.sub.Status = status
	return nil
}

func (r *refundSubscriptionRepo) Delete(context.Context, int64) error {
	now := time.Now()
	r.sub.DeletedAt = &now
	return nil
}

func (r *refundSubscriptionRepo) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return r.sub.DeletedAt == nil, nil
}

func (r *refundSubscriptionRepo) Restore(_ context.Context, _ int64, status string) (*UserSubscription, error) {
	r.restoreCalls++
	r.sub.DeletedAt = nil
	r.sub.Status = status
	copy := r.sub
	return &copy, nil
}

func TestRefundFailureReturnsHeldSubscription(t *testing.T) {
	for _, tc := range []struct {
		name          string
		remainingDays int
		wantRevoked   bool
	}{
		{name: "partial entitlement deduction", remainingDays: 40},
		{name: "full entitlement revocation", remainingDays: 5, wantRevoked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createCompletedRefundableOrderForTest(t, ctx, client, "subscription-held-refund")
			order, err := client.PaymentOrder.UpdateOneID(order.ID).
				SetOrderType(payment.OrderTypeSubscription).SetSubscriptionGroupID(7).SetSubscriptionDays(30).Save(ctx)
			require.NoError(t, err)
			expires := time.Now().AddDate(0, 0, tc.remainingDays).Truncate(time.Second)
			repo := &refundSubscriptionRepo{sub: UserSubscription{ID: 10, UserID: order.UserID,
				GroupID: 7, Status: SubscriptionStatusActive, ExpiresAt: expires}}
			groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 7}}
			prov := &pendingRefundProviderTestDouble{queryStatus: payment.ProviderStatusFailed}
			restore := replacePaymentProviderFactoryForTest(t, prov)
			defer restore()
			svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{},
				subscriptionSvc: NewSubscriptionService(groupRepo, repo, nil, nil, nil)}
			plan := &RefundPlan{OrderID: order.ID, Order: order, RefundAmount: 100, GatewayAmount: 100,
				DeductionType: payment.DeductionTypeSubscription, SubDaysToDeduct: 30, SubscriptionID: 10}
			result, err := svc.ExecuteRefund(ctx, plan)
			require.NoError(t, err)
			require.False(t, result.Success)
			require.Equal(t, tc.wantRevoked, repo.sub.DeletedAt != nil)
			detail, err := svc.latestRefundPendingDetail(ctx, order.ID)
			require.NoError(t, err)
			require.True(t, detail.DeductionHeld)
			require.Equal(t, tc.wantRevoked, detail.SubscriptionRevoked)
			result, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.NotContains(t, result.Warning, "rollback failed")
			require.Nil(t, repo.sub.DeletedAt)
			require.True(t, repo.sub.ExpiresAt.Equal(expires), "restore the original expiry")
			if tc.wantRevoked {
				require.Equal(t, 1, repo.restoreCalls)
			}
			_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.Error(t, err)
		})
	}
}
