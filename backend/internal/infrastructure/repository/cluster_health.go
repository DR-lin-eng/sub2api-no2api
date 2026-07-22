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

var _ service.ClusterHealthChecker = (*clusterHealthChecker)(nil)
