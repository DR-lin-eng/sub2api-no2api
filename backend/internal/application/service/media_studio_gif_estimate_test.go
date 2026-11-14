//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type image26EstimateGroups struct {
	GroupRepository
	group *Group
}

func (r image26EstimateGroups) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }
func (r image26EstimateGroups) ListActive(context.Context) ([]Group, error) {
	return []Group{*r.group}, nil
}

type image26EstimateUsers struct{ UserRepository }

func (image26EstimateUsers) GetByID(context.Context, int64) (*User, error) { return &User{ID: 1}, nil }

type image26EstimateSubscriptions struct{ UserSubscriptionRepository }

func (image26EstimateSubscriptions) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func TestMediaStudioGPTImage26FrameEstimateMatchesSettlement(t *testing.T) {
	price := 0.25
	frames := 4
	group := &Group{ID: 266, Platform: PlatformOpenAI, Status: StatusActive, RateMultiplier: 2, ImageCountBilling: true, ImagePrice1K: &price}
	groups := image26EstimateGroups{group: group}
	gateway := newOpenAIRecordUsageServiceWithBillingRepoForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageBillingRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	gateway.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, group.ID, "gpt-image-2.6-flare")
	studio := &MediaStudioService{groupRepo: groups, gatewayService: gateway,
		settingRepo:   &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{SettingKeyMediaStudioGroupRoutes: `[{"group_id":266,"media_type":"image","enabled":true,"models":["gpt-image-2.6-flare"]}]`}},
		apiKeyService: &APIKeyService{userRepo: image26EstimateUsers{}, groupRepo: groups, userSubRepo: image26EstimateSubscriptions{}},
	}
	estimate, err := studio.EstimateCost(t.Context(), 1, MediaStudioCostEstimateRequest{MediaType: "image", GroupID: group.ID, Model: "gpt-image-2.6-flare", Size: "1024x1024", Count: 2, OutputFormat: "gif", FrameCount: &frames})
	require.NoError(t, err)
	require.Equal(t, 8, estimate.ImageCount)
	require.InDelta(t, 4.0, estimate.Amount, 1e-8)
	_, err = studio.EstimateCost(t.Context(), 1, MediaStudioCostEstimateRequest{MediaType: "image", GroupID: group.ID, Model: "gpt-image-2.6-flare", Count: 2, OutputFormat: "svg"})
	require.Error(t, err, "SVG is a single generated file")
	_, err = studio.EstimateCost(t.Context(), 1, MediaStudioCostEstimateRequest{MediaType: "image", GroupID: group.ID, Model: "gpt-image-2.6-flare", Size: "auto", Count: 1, OutputFormat: "gif", FrameCount: &frames})
	require.ErrorIs(t, err, ErrMediaStudioCostEstimateUnavailable, "automatic GIF frame dimensions cannot establish a price tier before generation")
	group.ImageCountBilling = false
	_, err = studio.EstimateCost(t.Context(), 1, MediaStudioCostEstimateRequest{MediaType: "image", GroupID: group.ID, Model: "gpt-image-2.6-flare", Count: 1, OutputFormat: "gif", FrameCount: &frames})
	require.ErrorIs(t, err, ErrMediaStudioCostEstimateUnavailable, "token mode must wait for real usage")
}
