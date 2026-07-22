package admin

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

type accountStreamDegradationProviderStub struct {
	snapshot service.OpenAIStreamDegradationSnapshot
}

func (s *accountStreamDegradationProviderStub) BlockAccountScheduling(*service.Account, time.Time, string) {
}
func (s *accountStreamDegradationProviderStub) ClearAccountSchedulingBlock(int64) {}
func (s *accountStreamDegradationProviderStub) SnapshotOpenAIStreamDegradation(int64) (service.OpenAIStreamDegradationSnapshot, bool) {
	return s.snapshot, s.snapshot.Degraded
}

func TestBuildAccountResponseIncludesOpenAIStreamDegradation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	provider := &accountStreamDegradationProviderStub{snapshot: service.OpenAIStreamDegradationSnapshot{
		Degraded: true, Level: 3, ConsecutiveTimeouts: 4, DegradedSince: now, NextProbeAt: now.Add(80 * time.Second),
	}}
	rateLimit := &service.RateLimitService{}
	rateLimit.SetAccountRuntimeBlocker(provider)
	handler := &AccountHandler{rateLimitService: rateLimit}

	item := handler.buildAccountResponseWithRuntime(t.Context(), &service.Account{ID: 43, Platform: service.PlatformOpenAI})
	require.True(t, item.StreamDegraded)
	require.Equal(t, 3, item.StreamDegradationLevel)
	require.Equal(t, 4, item.StreamDegradationTimeouts)
	require.Equal(t, now, *item.StreamDegradedSince)
	require.Equal(t, now.Add(80*time.Second), *item.StreamNextProbeAt)
}
