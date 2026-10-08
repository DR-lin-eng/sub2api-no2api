package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestWSTurnRefreshKeepsSelectedGroupInMultipleBindings(t *testing.T) {
	selectedID, primaryID := int64(2), int64(1)
	selected := &service.Group{ID: selectedID, Platform: service.PlatformOpenAI, RateMultiplier: 3}
	conn := &service.APIKey{ID: 7, Key: "fixture", GroupID: &selectedID, Group: selected}
	updated := *selected
	updated.RateMultiplier = 0.3
	lookup := wsTurnAPIKeyLookupFunc(func(context.Context, string) (*service.APIKey, error) {
		return &service.APIKey{ID: conn.ID, GroupID: &primaryID,
			Group:         &service.Group{ID: primaryID, Platform: service.PlatformOpenAI},
			GroupBindings: []service.APIKeyGroupBinding{{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: selectedID}, Group: &updated}},
		}, nil
	})
	turn := refreshOpenAIWSTurnBillingAPIKey(context.Background(), lookup, conn)
	require.Equal(t, selectedID, *turn.GroupID)
	require.InDelta(t, 0.3, turn.Group.RateMultiplier, 0.00001)
	require.InDelta(t, 3, conn.Group.RateMultiplier, 0.00001)
}
