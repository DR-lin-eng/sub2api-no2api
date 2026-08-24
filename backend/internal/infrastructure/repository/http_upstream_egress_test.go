package repository

import (
	"net/http"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/stretchr/testify/require"
)

func (s *HTTPUpstreamSuite) TestIPv6RouteRotationEvictsOnlyStaleAccountRoutes() {
	svc := s.newService()
	oldRoute := platformegress.IPv6PoolRoute("2001:db8::10", 1, 1, false)
	newRoute := platformegress.IPv6PoolRoute("2001:db8::11", 1, 2, false)
	otherAccountRoute := platformegress.IPv6PoolRoute("2001:db8::20", 1, 1, false)
	svc.clients["old"] = &upstreamClientEntry{client: &http.Client{}, accountID: 7, routeKey: oldRoute.CacheKey(), routeMode: oldRoute.Mode}
	svc.clients["other"] = &upstreamClientEntry{client: &http.Client{}, accountID: 8, routeKey: otherAccountRoute.CacheKey(), routeMode: otherAccountRoute.Mode}

	svc.evictStaleAccountRoutesLocked(7, "", newRoute)

	require.NotContains(s.T(), svc.clients, "old")
	require.Contains(s.T(), svc.clients, "other")
}

func (s *HTTPUpstreamSuite) TestTraditionalAccountProxyRoutesRemainCached() {
	svc := s.newService()
	first := platformegress.ExternalProxyRoute("http://proxy-a:8080")
	second := platformegress.ExternalProxyRoute("http://proxy-b:8080")
	svc.clients["first"] = &upstreamClientEntry{client: &http.Client{}, accountID: 9, routeKey: first.CacheKey(), routeMode: first.Mode}

	svc.evictStaleAccountRoutesLocked(9, "", second)

	require.Contains(s.T(), svc.clients, "first")
}

func (s *HTTPUpstreamSuite) TestLeavingIPv6RouteEvictsItsIdlePool() {
	svc := s.newService()
	oldRoute := platformegress.IPv6PoolRoute("2001:db8::10", 1, 3, false)
	svc.clients["old"] = &upstreamClientEntry{client: &http.Client{}, accountID: 10, routeKey: oldRoute.CacheKey(), routeMode: oldRoute.Mode}

	svc.evictStaleAccountRoutesLocked(10, "", platformegress.ExternalProxyRoute("http://proxy.local:8080"))

	require.NotContains(s.T(), svc.clients, "old")
}
