package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func modelAttributeRuntimeFixture() *CustomModelRuntimeSnapshot {
	snapshot := customModelRuntimeFixture()
	template := snapshot.Templates[9]
	template.Attributes = map[string]any{"image": map[string]any{"fields": []any{
		map[string]any{"key": "quality", "type": "enum", "values": []any{"low", "high"}, "required": true},
	}}}
	snapshot.Templates[9] = template
	return snapshot
}

func TestModelAttributeRuntimeSnapshotRefresh(t *testing.T) {
	repo := &customModelConfigRepositoryStub{snapshot: modelAttributeRuntimeFixture()}
	svc := NewCustomModelConfigService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()
	fields, err := svc.ResolveModelAttributeFields(ctx, "exact-model", "image")
	require.NoError(t, err)
	require.NoError(t, validateModelAttributeValues(fields, map[string]any{"quality": "low"}))
	require.Error(t, validateModelAttributeValues(fields, map[string]any{"quality": "other"}))
	require.Error(t, validateModelAttributeValues(fields, nil))
	// Change the repository input, keeping the existing compiled snapshot alive.
	repo.snapshot.Templates[9].Attributes["image"] = map[string]any{"fields": []any{
		map[string]any{"key": "quality", "type": "enum", "values": []any{"other"}},
	}}
	cached, err := svc.ResolveModelAttributeFields(ctx, "exact-model", "image")
	require.NoError(t, err)
	require.NoError(t, validateModelAttributeValues(cached, map[string]any{"quality": "low"}))
	require.Equal(t, 1, repo.loads)
	svc.InvalidateRuntimeCache()
	refreshed, err := svc.ResolveModelAttributeFields(ctx, "exact-model", "image")
	require.NoError(t, err)
	require.Error(t, validateModelAttributeValues(refreshed, map[string]any{"quality": "low"}))
	require.NoError(t, validateModelAttributeValues(refreshed, map[string]any{"quality": "other"}))
	require.Equal(t, 2, repo.loads)
}

func BenchmarkModelAttributeRuntime(b *testing.B) {
	snapshot := modelAttributeRuntimeFixture()
	ctx := context.Background()
	b.Run("parse-per-request", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := modelAttributeFields(snapshot.Templates[9].Attributes, "image"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compiled-snapshot", func(b *testing.B) {
		repo := &customModelConfigRepositoryStub{snapshot: snapshot}
		svc := NewCustomModelConfigService(repo)
		svc.cacheTTL = time.Hour
		if _, err := svc.ResolveModelAttributeFields(ctx, "exact-model", "image"); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := svc.ResolveModelAttributeFields(ctx, "exact-model", "image"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
