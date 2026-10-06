package middleware

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestSystemOneAuthSelectsEligibleNativeBindingsInOriginalOrder(t *testing.T) {
	key := &service.APIKey{GroupBindings: []service.APIKeyGroupBinding{
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: 1}, Group: &service.Group{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive}},
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: 2}, Group: &service.Group{ID: 2, Platform: service.PlatformTypeSafe, Status: service.StatusActive}},
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: 3}, Group: &service.Group{ID: 3, Platform: service.PlatformOpenAI, Status: service.StatusActive}},
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: 4}, Group: &service.Group{ID: 4, Platform: service.PlatformComposite, Status: service.StatusActive}},
	}}
	copy := key.CloneForRequest()
	filterSystemOneGroupBindings(copy)
	ctx := service.WithAPIKeyGroupRouting(context.Background(), copy)
	require.NotNil(t, ctx)
	require.Equal(t, int64(2), *copy.GroupID)
	require.Equal(t, service.PlatformTypeSafe, copy.Group.Platform)
	require.Equal(t, []int64{2, 4}, []int64{copy.GroupBindings[0].GroupID, copy.GroupBindings[1].GroupID})
	require.Len(t, key.GroupBindings, 4, "shared auth cache entry must remain intact")
	require.Nil(t, key.GroupID)
}
