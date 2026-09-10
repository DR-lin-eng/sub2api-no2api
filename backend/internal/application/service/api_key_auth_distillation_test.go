package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshot_PreservesDistillationPolicy(t *testing.T) {
	group := &Group{ID: 31, Platform: PlatformAnthropic, Status: StatusActive, IsDistillationGroup: true}
	key := &APIKey{
		ID: 10, UserID: 20, GroupID: &group.ID,
		User: &User{ID: 20, Status: StatusActive}, Group: group,
		GroupBindings: []APIKeyGroupBinding{{
			APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: group.ID},
			Group:              group,
		}},
	}
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(t.Context(), key)
	wire, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var restored APIKeyAuthSnapshot
	require.NoError(t, json.Unmarshal(wire, &restored))
	materialized := svc.snapshotToAPIKey("sk-test", &restored)
	require.True(t, materialized.Group.IsDistillationGroup)
	require.Len(t, materialized.GroupBindings, 1)
	require.True(t, materialized.GroupBindings[0].Group.IsDistillationGroup)
}

func TestAPIKeyAuthSnapshot_RejectsV24WithoutDistillationPolicy(t *testing.T) {
	key, hit, err := (&APIKeyService{}).applyAuthCacheEntry("sk-test", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 24},
	})
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, key)
}
