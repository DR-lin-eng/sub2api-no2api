package service

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

// OpsOAuthPlanExpiryBucket is one remaining-time bucket for an OAuth account's
// subscription expiry. The order is stable so callers can render the result
// directly as a chart.
type OpsOAuthPlanExpiryBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// OpsOAuthPlanSummary reports how many OAuth accounts carry each plan marker.
// It is included alongside the expiry buckets so operators can see whether a
// distribution is dominated by one plan without exposing account credentials.
type OpsOAuthPlanSummary struct {
	PlanType string `json:"plan_type"`
	Count    int64  `json:"count"`
}

// OpsOAuthPlanExpiryDistribution is the current distribution of OAuth account
// subscription expiry dates. Expiry is read from credentials.subscription_expires_at;
// accounts without a valid value are counted separately.
type OpsOAuthPlanExpiryDistribution struct {
	GeneratedAt           time.Time                   `json:"generated_at"`
	Platform              string                      `json:"platform"`
	GroupID               *int64                      `json:"group_id"`
	TotalOAuthAccounts    int64                       `json:"total_oauth_accounts"`
	AccountsWithExpiry    int64                       `json:"accounts_with_expiry"`
	AccountsWithoutExpiry int64                       `json:"accounts_without_expiry"`
	Buckets               []*OpsOAuthPlanExpiryBucket `json:"buckets"`
	Plans                 []*OpsOAuthPlanSummary      `json:"plans"`
}

// opsOAuthPlanExpiryRepository is an optional narrow repository projection.
// The fallback in GetOAuthPlanExpiryDistribution keeps the service usable with
// older repository implementations and small unit-test stubs.
type opsOAuthPlanExpiryRepository interface {
	ListOAuthPlanExpiryAccounts(ctx context.Context, platform string, groupID *int64) ([]Account, error)
}

var oauthPlanExpiryBucketKeys = []string{
	"expired",
	"up_to_1",
	"up_to_2",
	"up_to_4",
	"up_to_5",
	"up_to_10",
	"up_to_15",
	"up_to_20",
	"up_to_25",
	"over_25",
}

// GetOAuthPlanExpiryDistribution returns OAuth-only subscription expiry
// distribution for the selected platform/group scope.
func (s *OpsService) GetOAuthPlanExpiryDistribution(
	ctx context.Context,
	platformFilter string,
	groupIDFilter *int64,
) (*OpsOAuthPlanExpiryDistribution, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}

	platformFilter = strings.ToLower(strings.TrimSpace(platformFilter))
	var (
		accounts []Account
		err      error
	)
	if repo, ok := s.accountRepo.(opsOAuthPlanExpiryRepository); ok {
		accounts, err = repo.ListOAuthPlanExpiryAccounts(ctx, platformFilter, groupIDFilter)
	} else if s.accountRepo != nil {
		groupID := int64(0)
		if groupIDFilter != nil {
			groupID = *groupIDFilter
		}
		accounts, err = s.accountRepo.ListAllWithFilters(ctx, platformFilter, AccountTypeOAuth, "", "", groupID, "")
	}
	if err != nil {
		return nil, err
	}

	return buildOAuthPlanExpiryDistribution(accounts, platformFilter, groupIDFilter, time.Now().UTC()), nil
}

func buildOAuthPlanExpiryDistribution(
	accounts []Account,
	platform string,
	groupID *int64,
	now time.Time,
) *OpsOAuthPlanExpiryDistribution {
	result := &OpsOAuthPlanExpiryDistribution{
		GeneratedAt: now.UTC(),
		Platform:    platform,
		GroupID:     groupID,
		Buckets:     make([]*OpsOAuthPlanExpiryBucket, 0, len(oauthPlanExpiryBucketKeys)),
		Plans:       make([]*OpsOAuthPlanSummary, 0),
	}
	for _, key := range oauthPlanExpiryBucketKeys {
		result.Buckets = append(result.Buckets, &OpsOAuthPlanExpiryBucket{Key: key})
	}

	planCounts := make(map[string]int64)
	for _, account := range accounts {
		if account.ID <= 0 || account.Type != AccountTypeOAuth || account.ParentAccountID != nil {
			continue
		}
		result.TotalOAuthAccounts++

		planType := strings.TrimSpace(account.GetCredential("plan_type"))
		if planType == "" {
			planType = "unknown"
		}
		planCounts[planType]++

		expiresAt := account.GetCredentialAsTime("subscription_expires_at")
		if expiresAt == nil {
			result.AccountsWithoutExpiry++
			continue
		}
		result.AccountsWithExpiry++

		bucketIndex := oauthPlanExpiryBucketIndex(*expiresAt, now)
		result.Buckets[bucketIndex].Count++
	}

	for planType, count := range planCounts {
		result.Plans = append(result.Plans, &OpsOAuthPlanSummary{PlanType: planType, Count: count})
	}
	sort.Slice(result.Plans, func(i, j int) bool {
		if result.Plans[i].Count != result.Plans[j].Count {
			return result.Plans[i].Count > result.Plans[j].Count
		}
		return result.Plans[i].PlanType < result.Plans[j].PlanType
	})
	return result
}

func oauthPlanExpiryBucketIndex(expiresAt, now time.Time) int {
	if !expiresAt.After(now) {
		return 0
	}
	remainingDays := int(math.Ceil(expiresAt.Sub(now).Hours() / 24))
	for i, threshold := range []int{1, 2, 4, 5, 10, 15, 20, 25} {
		if remainingDays <= threshold {
			return i + 1
		}
	}
	return len(oauthPlanExpiryBucketKeys) - 1
}
