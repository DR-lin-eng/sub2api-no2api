package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type compositeRouteRepoStub struct {
	routes []CompositeModelRoute
}

type countingCompositeRouteRepo struct {
	mu        sync.RWMutex
	routes    []CompositeModelRoute
	loadDelay time.Duration
	loads     atomic.Int64
}

func (r *countingCompositeRouteRepo) setRoutes(routes []CompositeModelRoute) {
	r.mu.Lock()
	r.routes = append([]CompositeModelRoute(nil), routes...)
	r.mu.Unlock()
}

func (r *countingCompositeRouteRepo) ListByGroup(_ context.Context, groupID int64, includeDisabled bool) ([]CompositeModelRoute, error) {
	r.loads.Add(1)
	if r.loadDelay > 0 {
		time.Sleep(r.loadDelay)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	routes := make([]CompositeModelRoute, 0, len(r.routes))
	for i := range r.routes {
		if r.routes[i].GroupID != groupID || (!includeDisabled && !r.routes[i].Enabled) {
			continue
		}
		routes = append(routes, r.routes[i])
	}
	return routes, nil
}

func (r *countingCompositeRouteRepo) Create(context.Context, *CompositeModelRoute) error {
	return nil
}

func (r *countingCompositeRouteRepo) Update(context.Context, *CompositeModelRoute) error {
	return nil
}

func (r *countingCompositeRouteRepo) Delete(context.Context, int64) error {
	return nil
}

func (r *countingCompositeRouteRepo) DeleteByGroup(context.Context, int64) error {
	return nil
}

func (s compositeRouteRepoStub) ListByGroup(ctx context.Context, groupID int64, includeDisabled bool) ([]CompositeModelRoute, error) {
	routes := make([]CompositeModelRoute, 0, len(s.routes))
	for _, route := range s.routes {
		if route.GroupID != groupID {
			continue
		}
		if !includeDisabled && !route.Enabled {
			continue
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s compositeRouteRepoStub) Create(ctx context.Context, route *CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Update(ctx context.Context, route *CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

func (s compositeRouteRepoStub) DeleteByGroup(ctx context.Context, groupID int64) error {
	return nil
}

func TestCompositeRouteResolverExplicitExactRouteRewritesModel(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             10,
				GroupID:        7,
				PublicModel:    "openrouter/gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "openrouter/gpt-5", CompositeRouteEndpointChatCompletions)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5", decision.UpstreamModel)
	require.NotNil(t, decision.Route)
	require.Equal(t, int64(10), decision.Route.ID)
}

func TestCompositeRouteResolverPrefersEndpointSpecificLongestPrefix(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "router/",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformAnthropic,
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       10,
				Enabled:        true,
			},
			{
				ID:             2,
				GroupID:        7,
				PublicModel:    "router/gpt-",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-family",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "router/gpt-5", CompositeRouteEndpointResponses)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-family", decision.UpstreamModel)
	require.NotNil(t, decision.Route)
	require.Equal(t, int64(2), decision.Route.ID)
}

func TestCompositeRouteResolverIgnoresDisabledRoutesAndFallsBackToDetector(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformAnthropic,
				UpstreamModel:  "claude-sonnet-4-6",
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        false,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gpt-5", CompositeRouteEndpointAny)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceDetector, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5", decision.UpstreamModel)
	require.Nil(t, decision.Route)
}

func TestCompositeRouteResolverExplicitRoutesCoverBucketTwoProviders(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "all/gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             2,
				GroupID:        7,
				PublicModel:    "all/claude-sonnet",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformAnthropic,
				UpstreamModel:  "claude-sonnet-4-6",
				Endpoint:       CompositeRouteEndpointMessages,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             3,
				GroupID:        7,
				PublicModel:    "all/gemini-pro",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformGemini,
				UpstreamModel:  "gemini-2.5-pro",
				Endpoint:       CompositeRouteEndpointGemini,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             4,
				GroupID:        7,
				PublicModel:    "all/grok",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformGrok,
				UpstreamModel:  "grok-4.3",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	tests := []struct {
		model        string
		endpoint     string
		wantPlatform string
		wantUpstream string
	}{
		{"all/gpt-5", CompositeRouteEndpointResponses, PlatformOpenAI, "gpt-5"},
		{"all/claude-sonnet", CompositeRouteEndpointMessages, PlatformAnthropic, "claude-sonnet-4-6"},
		{"all/gemini-pro", CompositeRouteEndpointGemini, PlatformGemini, "gemini-2.5-pro"},
		{"all/grok", CompositeRouteEndpointResponses, PlatformGrok, "grok-4.3"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			decision, err := resolver.Resolve(context.Background(), 7, tt.model, tt.endpoint)

			require.NoError(t, err)
			require.True(t, decision.Matched)
			require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
			require.Equal(t, tt.wantPlatform, decision.TargetPlatform)
			require.Equal(t, tt.wantUpstream, decision.UpstreamModel)
		})
	}
}

func TestCompositeRouteResolverCachesAndInvalidatesSnapshot(t *testing.T) {
	repo := &countingCompositeRouteRepo{}
	repo.setRoutes([]CompositeModelRoute{{
		ID:             1,
		GroupID:        7,
		PublicModel:    "public-model",
		MatchType:      CompositeRouteMatchExact,
		TargetPlatform: PlatformOpenAI,
		UpstreamModel:  "gpt-before",
		Endpoint:       CompositeRouteEndpointResponses,
		Enabled:        true,
	}})
	resolver := newCompositeRouteResolver(repo, time.Hour)

	for i := 0; i < 2; i++ {
		decision, err := resolver.Resolve(context.Background(), 7, "public-model", CompositeRouteEndpointResponses)
		require.NoError(t, err)
		require.Equal(t, "gpt-before", decision.UpstreamModel)
	}
	require.Equal(t, int64(1), repo.loads.Load())

	repo.setRoutes([]CompositeModelRoute{{
		ID:             2,
		GroupID:        7,
		PublicModel:    "public-model",
		MatchType:      CompositeRouteMatchExact,
		TargetPlatform: PlatformOpenAI,
		UpstreamModel:  "gpt-after",
		Endpoint:       CompositeRouteEndpointResponses,
		Enabled:        true,
	}})
	resolver.Invalidate(7)

	decision, err := resolver.Resolve(context.Background(), 7, "public-model", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.Equal(t, "gpt-after", decision.UpstreamModel)
	require.Equal(t, int64(2), repo.loads.Load())
}

func TestCompositeRouteResolverCoalescesConcurrentColdLoads(t *testing.T) {
	repo := &countingCompositeRouteRepo{loadDelay: 20 * time.Millisecond}
	repo.setRoutes([]CompositeModelRoute{{
		ID:             1,
		GroupID:        7,
		PublicModel:    "public-model",
		MatchType:      CompositeRouteMatchExact,
		TargetPlatform: PlatformOpenAI,
		UpstreamModel:  "gpt-5.6",
		Endpoint:       CompositeRouteEndpointResponses,
		Enabled:        true,
	}})
	resolver := newCompositeRouteResolver(repo, time.Hour)

	const workers = 32
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			decision, err := resolver.Resolve(context.Background(), 7, "public-model", CompositeRouteEndpointResponses)
			if err != nil {
				errs <- err
				return
			}
			if !decision.Matched || decision.UpstreamModel != "gpt-5.6" {
				errs <- fmt.Errorf("unexpected decision: %+v", decision)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), repo.loads.Load())
}

func BenchmarkCompositeRouteResolverResolveExplicitRoute(b *testing.B) {
	routes := make([]CompositeModelRoute, 0, 18)
	for i := 0; i < 16; i++ {
		routes = append(routes, CompositeModelRoute{
			ID:             int64(i + 1),
			GroupID:        7,
			PublicModel:    "router/model-" + string(rune('a'+i)),
			MatchType:      CompositeRouteMatchPrefix,
			TargetPlatform: PlatformOpenAI,
			Endpoint:       CompositeRouteEndpointAny,
			Priority:       100 + i,
			Enabled:        true,
		})
	}
	routes = append(routes,
		CompositeModelRoute{
			ID:             100,
			GroupID:        7,
			PublicModel:    "router/gpt-",
			MatchType:      CompositeRouteMatchPrefix,
			TargetPlatform: PlatformOpenAI,
			UpstreamModel:  "gpt-5.6",
			Endpoint:       CompositeRouteEndpointResponses,
			Priority:       10,
			Enabled:        true,
		},
		CompositeModelRoute{
			ID:             101,
			GroupID:        7,
			PublicModel:    "router/gpt-5.6",
			MatchType:      CompositeRouteMatchExact,
			TargetPlatform: PlatformOpenAI,
			UpstreamModel:  "gpt-5.6",
			Endpoint:       CompositeRouteEndpointResponses,
			Priority:       20,
			Enabled:        true,
		},
	)

	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})
	ctx := context.Background()
	if _, err := resolver.Resolve(ctx, 7, "router/gpt-5.6", CompositeRouteEndpointResponses); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision, err := resolver.Resolve(ctx, 7, "router/gpt-5.6", CompositeRouteEndpointResponses)
		if err != nil || !decision.Matched {
			b.Fatalf("unexpected decision: %+v, err=%v", decision, err)
		}
	}
}
