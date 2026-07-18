package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const defaultCompositeRouteSnapshotTTL = 30 * time.Second

type compositeRouteExactKey struct {
	endpoint string
	model    string
}

type compositeRouteSnapshot struct {
	exact    map[compositeRouteExactKey]*CompositeModelRoute
	prefixes map[string][]CompositeModelRoute
}

type compositeRouteCacheEntry struct {
	snapshot  *compositeRouteSnapshot
	expiresAt int64
}

type CompositeRouteResolver struct {
	repo       CompositeModelRouteRepository
	cacheTTL   time.Duration
	cache      sync.Map // map[int64]*compositeRouteCacheEntry
	generation sync.Map // map[int64]*atomic.Uint64
	loadGroup  singleflight.Group
}

func NewCompositeRouteResolver(repo CompositeModelRouteRepository) *CompositeRouteResolver {
	return newCompositeRouteResolver(repo, defaultCompositeRouteSnapshotTTL)
}

func newCompositeRouteResolver(repo CompositeModelRouteRepository, cacheTTL time.Duration) *CompositeRouteResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultCompositeRouteSnapshotTTL
	}
	return &CompositeRouteResolver{repo: repo, cacheTTL: cacheTTL}
}

// Invalidate drops a group's immutable route snapshot after an admin mutation.
// Other instances converge through the short TTL without adding a database read
// to the request hot path.
func (r *CompositeRouteResolver) Invalidate(groupID int64) {
	if r == nil || groupID <= 0 {
		return
	}
	r.groupGeneration(groupID).Add(1)
	r.cache.Delete(groupID)
	r.loadGroup.Forget(strconv.FormatInt(groupID, 10))
}

func (r *CompositeRouteResolver) groupGeneration(groupID int64) *atomic.Uint64 {
	value, _ := r.generation.LoadOrStore(groupID, &atomic.Uint64{})
	counter, ok := value.(*atomic.Uint64)
	if !ok {
		panic("invalid composite route generation entry")
	}
	return counter
}

func (r *CompositeRouteResolver) Resolve(ctx context.Context, groupID int64, model, endpoint string) (CompositeRouteDecision, error) {
	model = strings.TrimSpace(model)
	endpoint = normalizeCompositeRouteEndpoint(endpoint)
	decision := CompositeRouteDecision{
		GroupID:     groupID,
		PublicModel: model,
		Endpoint:    endpoint,
	}
	if model == "" {
		decision.Reason = "model is required"
		return decision, nil
	}

	if r != nil && r.repo != nil && groupID > 0 {
		snapshot, err := r.loadSnapshot(ctx, groupID)
		if err != nil {
			return decision, fmt.Errorf("list composite routes: %w", err)
		}
		if route, ok := snapshot.match(model, endpoint); ok {
			upstreamModel := strings.TrimSpace(route.UpstreamModel)
			if upstreamModel == "" {
				upstreamModel = model
			}
			return CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceExplicit,
				GroupID:        groupID,
				PublicModel:    model,
				TargetPlatform: route.TargetPlatform,
				UpstreamModel:  upstreamModel,
				Endpoint:       endpoint,
				Route:          route,
			}, nil
		}
	}

	if platform, ok := DetectModelPlatform(model); ok {
		return CompositeRouteDecision{
			Matched:        true,
			Source:         CompositeRouteSourceDetector,
			GroupID:        groupID,
			PublicModel:    model,
			TargetPlatform: platform,
			UpstreamModel:  model,
			Endpoint:       endpoint,
		}, nil
	}
	decision.Reason = "no explicit route or built-in detector match"
	return decision, nil
}

func (r *CompositeRouteResolver) loadSnapshot(ctx context.Context, groupID int64) (*compositeRouteSnapshot, error) {
	now := time.Now().UnixNano()
	if cached, ok := r.cache.Load(groupID); ok {
		entry, valid := cached.(*compositeRouteCacheEntry)
		if valid && now < entry.expiresAt {
			return entry.snapshot, nil
		}
		r.cache.CompareAndDelete(groupID, cached)
	}

	key := strconv.FormatInt(groupID, 10)
	generation := r.groupGeneration(groupID).Load()
	loaded, err, _ := r.loadGroup.Do(key, func() (any, error) {
		now := time.Now().UnixNano()
		if cached, ok := r.cache.Load(groupID); ok {
			entry, valid := cached.(*compositeRouteCacheEntry)
			if valid && now < entry.expiresAt {
				return entry.snapshot, nil
			}
			r.cache.CompareAndDelete(groupID, cached)
		}

		routes, err := r.repo.ListByGroup(ctx, groupID, false)
		if err != nil {
			return nil, err
		}
		snapshot := compileCompositeRouteSnapshot(routes)
		if r.groupGeneration(groupID).Load() == generation {
			r.cache.Store(groupID, &compositeRouteCacheEntry{
				snapshot:  snapshot,
				expiresAt: time.Now().Add(r.cacheTTL).UnixNano(),
			})
		}
		return snapshot, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot, ok := loaded.(*compositeRouteSnapshot)
	if !ok {
		return nil, fmt.Errorf("invalid composite route snapshot result")
	}
	return snapshot, nil
}

func compileCompositeRouteSnapshot(routes []CompositeModelRoute) *compositeRouteSnapshot {
	snapshot := &compositeRouteSnapshot{
		exact:    make(map[compositeRouteExactKey]*CompositeModelRoute, len(routes)),
		prefixes: make(map[string][]CompositeModelRoute),
	}
	for i := range routes {
		route := routes[i]
		if !route.Enabled {
			continue
		}
		route.PublicModel = strings.TrimSpace(route.PublicModel)
		if route.PublicModel == "" {
			continue
		}
		route.MatchType = normalizeCompositeRouteMatchType(route.MatchType)
		route.Endpoint = normalizeCompositeRouteEndpoint(route.Endpoint)
		route.TargetPlatform = strings.TrimSpace(route.TargetPlatform)
		route.UpstreamModel = strings.TrimSpace(route.UpstreamModel)

		if route.MatchType == CompositeRouteMatchExact {
			key := compositeRouteExactKey{endpoint: route.Endpoint, model: route.PublicModel}
			if current := snapshot.exact[key]; current == nil || compositeRouteStablePrecedes(route, *current) {
				copyRoute := route
				snapshot.exact[key] = &copyRoute
			}
			continue
		}
		snapshot.prefixes[route.Endpoint] = append(snapshot.prefixes[route.Endpoint], route)
	}

	for endpoint := range snapshot.prefixes {
		prefixes := snapshot.prefixes[endpoint]
		sort.Slice(prefixes, func(i, j int) bool {
			if len(prefixes[i].PublicModel) != len(prefixes[j].PublicModel) {
				return len(prefixes[i].PublicModel) > len(prefixes[j].PublicModel)
			}
			return compositeRouteStablePrecedes(prefixes[i], prefixes[j])
		})
		snapshot.prefixes[endpoint] = prefixes
	}
	return snapshot
}

func (s *compositeRouteSnapshot) match(model, endpoint string) (*CompositeModelRoute, bool) {
	if s == nil {
		return nil, false
	}
	if route := s.exact[compositeRouteExactKey{endpoint: endpoint, model: model}]; route != nil {
		return route, true
	}
	if endpoint != CompositeRouteEndpointAny {
		if route := s.exact[compositeRouteExactKey{endpoint: CompositeRouteEndpointAny, model: model}]; route != nil {
			return route, true
		}
	}
	if route := matchCompositePrefixSnapshot(s.prefixes[endpoint], model); route != nil {
		return route, true
	}
	if endpoint != CompositeRouteEndpointAny {
		if route := matchCompositePrefixSnapshot(s.prefixes[CompositeRouteEndpointAny], model); route != nil {
			return route, true
		}
	}
	return nil, false
}

func matchCompositePrefixSnapshot(routes []CompositeModelRoute, model string) *CompositeModelRoute {
	for i := range routes {
		if strings.HasPrefix(model, routes[i].PublicModel) {
			return &routes[i]
		}
	}
	return nil
}

func compositeRouteStablePrecedes(a, b CompositeModelRoute) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	return a.ID < b.ID
}
