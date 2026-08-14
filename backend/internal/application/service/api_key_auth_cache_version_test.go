package service

import (
	"encoding/json"
	"testing"
)

func TestAPIKeyService_RejectsV10AuthSnapshotWithoutModelsListConfig(t *testing.T) {
	groupID := int64(9)
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-models-list", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  10,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:          2,
				Status:      StatusActive,
				Role:        RoleUser,
				Balance:     10,
				Concurrency: 3,
			},
			Group: &APIKeyAuthGroupSnapshot{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatalf("expected v10 auth snapshot to be rejected after models_list_config was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV17AuthSnapshotWithoutReasoningEffortPolicy(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-reasoning-mappings", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 17},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v17 auth snapshot to be rejected after reasoning effort policy was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV18AuthSnapshotWithoutRequestSchedulingTier(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-scheduling-tier", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 18},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v18 auth snapshot to be rejected after request scheduling tier was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RoundTripsRequestSchedulingTierInAuthSnapshot(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{
		ID:     10,
		UserID: 20,
		User: &User{
			ID:             20,
			Status:         StatusActive,
			SchedulingTier: RequestSchedulingTierLow,
		},
	}

	snapshot := svc.snapshotFromAPIKey(t.Context(), key)
	if snapshot == nil || snapshot.User.SchedulingTier != RequestSchedulingTierLow {
		t.Fatalf("expected low tier in snapshot, got %#v", snapshot)
	}
	roundTripped := svc.snapshotToAPIKey("sk-test", snapshot)
	if roundTripped == nil || roundTripped.User == nil || roundTripped.User.SchedulingTier != RequestSchedulingTierLow {
		t.Fatalf("expected low tier after snapshot materialization, got %#v", roundTripped)
	}
}

func TestAPIKeyService_RoundTripsGroupPricingInAuthSnapshot(t *testing.T) {
	groupID := int64(30)
	inputPrice := 9.99
	outputPrice := 19.99
	svc := &APIKeyService{}
	key := &APIKey{
		ID:      10,
		UserID:  20,
		GroupID: &groupID,
		User: &User{
			ID:     20,
			Status: StatusActive,
		},
		Group: &Group{
			ID:                        groupID,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Models:      []string{"some-model"},
				BillingMode: BillingModeToken,
				InputPrice:  &inputPrice,
				OutputPrice: &outputPrice,
			}},
		},
	}

	snapshot := svc.snapshotFromAPIKey(t.Context(), key)
	if snapshot == nil || snapshot.Group == nil || !snapshot.Group.LongContextPricingEnabled || len(snapshot.Group.ModelPricing) != 1 {
		t.Fatalf("expected group pricing fields in auth snapshot, got %#v", snapshot)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal auth snapshot: %v", err)
	}
	var cached APIKeyAuthSnapshot
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatalf("unmarshal auth snapshot: %v", err)
	}
	roundTripped := svc.snapshotToAPIKey("sk-test", &cached)
	if roundTripped == nil || roundTripped.Group == nil || !roundTripped.Group.LongContextPricingEnabled {
		t.Fatalf("expected long-context pricing gate after snapshot materialization, got %#v", roundTripped)
	}
	if len(roundTripped.Group.ModelPricing) != 1 || roundTripped.Group.ModelPricing[0].InputPrice == nil ||
		*roundTripped.Group.ModelPricing[0].InputPrice != inputPrice || roundTripped.Group.ModelPricing[0].OutputPrice == nil ||
		*roundTripped.Group.ModelPricing[0].OutputPrice != outputPrice {
		t.Fatalf("expected group model pricing after snapshot materialization, got %#v", roundTripped.Group.ModelPricing)
	}
}
