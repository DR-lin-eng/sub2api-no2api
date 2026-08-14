package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

type clusterHealthChecker struct {
	rdb *redis.Client
}

func NewClusterHealthChecker(rdb *redis.Client) service.ClusterHealthChecker {
	return &clusterHealthChecker{rdb: rdb}
}

func (c *clusterHealthChecker) RedisHealthy(ctx context.Context) bool {
	return c != nil && c.rdb != nil && c.rdb.Ping(ctx).Err() == nil
}

func (c *clusterHealthChecker) ClusterConnectionStats() service.ClusterConnectionStats {
	if c == nil || c.rdb == nil {
		return service.ClusterConnectionStats{}
	}
	stats := c.rdb.PoolStats()
	active := int(stats.TotalConns) - int(stats.IdleConns)
	if active < 0 {
		active = 0
	}
	return service.ClusterConnectionStats{
		Active: active,
		Idle:   int(stats.IdleConns),
		Max:    c.rdb.Options().PoolSize,
	}
}

var _ service.ClusterHealthChecker = (*clusterHealthChecker)(nil)
