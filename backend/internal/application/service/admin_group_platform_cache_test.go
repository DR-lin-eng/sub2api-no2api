//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type groupPlatformRepoStub struct {
	GroupRepository
	group   *Group
	updated *Group
}

func (r *groupPlatformRepoStub) GetByID(context.Context, int64) (*Group, error) {
	cloned := *r.group
	return &cloned, nil
}

func (r *groupPlatformRepoStub) Update(_ context.Context, group *Group) error {
	r.updated = group
	return nil
}

type channelCacheInvalidatorSpy struct{ calls int }

func (s *channelCacheInvalidatorSpy) InvalidateCache() { s.calls++ }

func TestUpdateGroupInvalidatesChannelCacheOnPlatformChange(t *testing.T) {
	tests := []struct {
		name          string
		fromPlatform  string
		inputPlatform string
		wantCalls     int
	}{
		{"platform changed", PlatformAnthropic, PlatformOpenAI, 1},
		{"same platform", PlatformAnthropic, PlatformAnthropic, 0},
		{"platform omitted", PlatformAnthropic, "", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &groupPlatformRepoStub{group: &Group{ID: 7, Name: "g", Platform: tc.fromPlatform}}
			spy := &channelCacheInvalidatorSpy{}
			svc := &adminServiceImpl{groupRepo: repo, channelCacheInvalidator: spy}

			got, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{Platform: tc.inputPlatform})
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tc.wantCalls, spy.calls)
		})
	}
}

func TestUpdateGroupWithoutChannelCacheInvalidator(t *testing.T) {
	repo := &groupPlatformRepoStub{group: &Group{ID: 7, Name: "g", Platform: PlatformAnthropic}}
	svc := &adminServiceImpl{groupRepo: repo}

	got, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{Platform: PlatformOpenAI})
	require.NoError(t, err)
	require.Equal(t, PlatformOpenAI, got.Platform)
}
