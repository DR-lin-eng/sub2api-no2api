package service

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func distillationTestContext(group *Group) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("api_key", &APIKey{Group: group, GroupID: &group.ID})
	return c
}

func TestStripDistillationCacheFields_RemovesNestedCacheSignals(t *testing.T) {
	body := []byte(`{"model":"claude","prompt_cache_key":"root","prompt_cache_retention":"1h","system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}],"messages":[{"content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral","ttl":"5m"}}]}],"tools":[{"name":"search","cache_control":{"type":"ephemeral"}}],"metadata":{"keep":"yes"}}`)

	cleaned := stripDistillationCacheFields(body)
	require.NotContains(t, string(cleaned), "prompt_cache_key")
	require.NotContains(t, string(cleaned), "prompt_cache_retention")
	require.NotContains(t, string(cleaned), "cache_control")
	require.Contains(t, string(cleaned), `"keep":"yes"`)
}

func TestDistillationSessionID_RotatesAtTenThousandRequests(t *testing.T) {
	group := &Group{ID: 910001, IsDistillationGroup: true}
	account := &Account{ID: 920001, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc := &GatewayService{}

	var first string
	for request := 1; request <= 10000; request++ {
		got, ok := svc.DistillationSessionID(context.Background(), distillationTestContext(group), account)
		require.True(t, ok)
		if request == 1 {
			first = got
		}
		require.Equal(t, first, got)
	}

	rotated, ok := svc.DistillationSessionID(context.Background(), distillationTestContext(group), account)
	require.True(t, ok)
	require.NotEqual(t, first, rotated)
}

func TestDistillationSessionID_ReusesRequestLocalState(t *testing.T) {
	group := &Group{ID: 930001, IsDistillationGroup: true}
	account := &Account{ID: 940001, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
	svc := &GatewayService{}
	c := distillationTestContext(group)

	first, ok := svc.DistillationSessionID(context.Background(), c, account)
	require.True(t, ok)
	second, ok := svc.DistillationSessionID(context.Background(), c, account)
	require.True(t, ok)
	require.Equal(t, first, second)
}

func TestIsDistillationGroupRequest_OnlyAnthropicOAuthAccounts(t *testing.T) {
	group := &Group{ID: 950001, IsDistillationGroup: true}
	svc := &GatewayService{}
	c := distillationTestContext(group)

	require.True(t, svc.IsDistillationGroupRequest(c, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
	require.True(t, svc.IsDistillationGroupRequest(c, &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}))
	require.False(t, svc.IsDistillationGroupRequest(c, &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
	require.False(t, svc.IsDistillationGroupRequest(c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
}
