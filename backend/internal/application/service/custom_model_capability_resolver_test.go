package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type customModelConfigRepositoryStub struct {
	snapshot *CustomModelRuntimeSnapshot
	loadErr  error
	loads    int
}

func (s *customModelConfigRepositoryStub) LoadRuntimeSnapshot(context.Context) (*CustomModelRuntimeSnapshot, error) {
	s.loads++
	return s.snapshot, s.loadErr
}

func (s *customModelConfigRepositoryStub) ListConfigs(context.Context) ([]CustomModelConfig, error) {
	return append([]CustomModelConfig(nil), s.snapshot.Configs...), nil
}

func (s *customModelConfigRepositoryStub) GetConfig(context.Context, int64) (*CustomModelConfig, error) {
	return nil, ErrCustomModelConfigNotFound
}

func (s *customModelConfigRepositoryStub) CreateConfig(
	_ context.Context,
	input CreateCustomModelConfigInput,
) (*CustomModelConfig, error) {
	return &CustomModelConfig{
		ID:           1,
		ModelName:    input.ModelName,
		PrefixMatch:  input.PrefixMatch,
		Capabilities: input.Capabilities,
		VideoAPIType: input.VideoAPIType,
		TemplateID:   input.TemplateID,
	}, nil
}

func (s *customModelConfigRepositoryStub) UpdateConfig(
	context.Context,
	int64,
	UpdateCustomModelConfigInput,
) (*CustomModelConfig, error) {
	return nil, ErrCustomModelConfigNotFound
}

func (s *customModelConfigRepositoryStub) DeleteConfig(context.Context, int64) error {
	return nil
}

func (s *customModelConfigRepositoryStub) ListTemplates(context.Context) ([]CustomModelRequestTemplate, error) {
	return nil, nil
}

func (s *customModelConfigRepositoryStub) GetTemplate(
	_ context.Context,
	id int64,
) (*CustomModelRequestTemplate, error) {
	item, ok := s.snapshot.Templates[id]
	if !ok {
		return nil, ErrCustomModelTemplateNotFound
	}
	return &item, nil
}

func (s *customModelConfigRepositoryStub) CreateTemplate(
	context.Context,
	CreateCustomModelRequestTemplateInput,
) (*CustomModelRequestTemplate, error) {
	return nil, nil
}

func (s *customModelConfigRepositoryStub) UpdateTemplate(
	context.Context,
	int64,
	UpdateCustomModelRequestTemplateInput,
) (*CustomModelRequestTemplate, error) {
	return nil, nil
}

func (s *customModelConfigRepositoryStub) DeleteTemplate(context.Context, int64) error {
	return nil
}

func customModelRuntimeFixture() *CustomModelRuntimeSnapshot {
	templateID := int64(9)
	return &CustomModelRuntimeSnapshot{
		Enabled: true,
		Configs: []CustomModelConfig{
			{ModelName: "exact-model", Capabilities: []string{"image"}, TemplateID: &templateID},
			{ModelName: "vendor-", PrefixMatch: true, Capabilities: []string{"audio"}},
			{ModelName: "vendor-video-", PrefixMatch: true, Capabilities: []string{"video"}, VideoAPIType: "agnes"},
		},
		Templates: map[int64]CustomModelRequestTemplate{
			9: {ID: 9, RequestAdapter: map[string]any{"version": float64(1)}},
		},
	}
}

func TestCustomModelRuntimeCachesAndUsesLongestPrefix(t *testing.T) {
	repo := &customModelConfigRepositoryStub{snapshot: customModelRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()

	image, err := svc.HasCapability(ctx, "EXACT-MODEL", "image")
	require.NoError(t, err)
	require.True(t, image)
	video, err := svc.HasCapability(ctx, "vendor-video-v2", "video")
	require.NoError(t, err)
	require.True(t, video)
	audio, err := svc.HasCapability(ctx, "vendor-video-v2", "audio")
	require.NoError(t, err)
	require.False(t, audio, "the longest prefix must win")
	require.Equal(t, 1, repo.loads)
}

func TestCustomModelRuntimeCapabilityAndAdapterCache(t *testing.T) {
	repo := &customModelConfigRepositoryStub{snapshot: customModelRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()

	videoType, configured, err := svc.ResolveVideoAPIType(ctx, "vendor-video-v2")
	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, "agnes", videoType)
	audio, err := svc.HasCapability(ctx, "vendor-audio-v1", "audio")
	require.NoError(t, err)
	require.True(t, audio)
	adapter, ok, err := svc.ResolveRequestAdapter(ctx, "exact-model")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, float64(1), adapter["version"])
	require.Equal(t, 1, repo.loads)

	svc.InvalidateRuntimeCache()
	_, _, err = svc.ResolveRequestAdapter(ctx, "exact-model")
	require.NoError(t, err)
	require.Equal(t, 2, repo.loads)
}

func TestCustomModelRuntimeListIsEmptyWhileFeatureIsDisabled(t *testing.T) {
	snapshot := customModelRuntimeFixture()
	snapshot.Enabled = false
	repo := &customModelConfigRepositoryStub{snapshot: snapshot}
	svc := NewCustomModelConfigService(repo)

	items, err := svc.ListEnabled(context.Background())
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestCustomModelRuntimeKeepsLastSnapshotOnRefreshFailure(t *testing.T) {
	repo := &customModelConfigRepositoryStub{snapshot: customModelRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)
	now := time.Unix(100, 0)
	svc.now = func() time.Time { return now }
	svc.cacheTTL = time.Second

	matched, err := svc.HasCapability(context.Background(), "exact-model", "image")
	require.NoError(t, err)
	require.True(t, matched)
	now = now.Add(2 * time.Second)
	repo.loadErr = errors.New("database unavailable")

	matched, err = svc.HasCapability(context.Background(), "exact-model", "image")
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, 2, repo.loads)
}

func TestCustomModelConfigCreateNormalizesAndRejectsInvalidCapabilities(t *testing.T) {
	repo := &customModelConfigRepositoryStub{snapshot: customModelRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)

	_, err := svc.Create(context.Background(), CreateCustomModelConfigInput{
		ModelName:    "new-model",
		Capabilities: []string{"unknown"},
	})
	require.ErrorIs(t, err, ErrCustomModelConfigInvalid)

	created, err := svc.Create(context.Background(), CreateCustomModelConfigInput{
		ModelName:    "  new-model  ",
		Capabilities: []string{"IMAGE", "image"},
		VideoAPIType: "agnes",
	})
	require.NoError(t, err)
	require.Equal(t, "new-model", created.ModelName)
	require.Equal(t, []string{"image"}, created.Capabilities)
	require.Empty(t, created.VideoAPIType)
}

func BenchmarkCustomModelCapabilityCached(b *testing.B) {
	repo := &customModelConfigRepositoryStub{snapshot: customModelRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()
	_, _ = svc.HasCapability(ctx, "vendor-video-v2", "video")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		matched, err := svc.HasCapability(ctx, "vendor-video-v2", "video")
		if err != nil || !matched {
			b.Fatalf("unexpected resolution: matched=%v err=%v", matched, err)
		}
	}
}
