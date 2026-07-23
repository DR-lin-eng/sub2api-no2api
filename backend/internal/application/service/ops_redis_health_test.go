package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseOpsMetricsCollectorInterval(t *testing.T) {
	require.Equal(t, time.Minute, parseOpsMetricsCollectorInterval(""))
	require.Equal(t, time.Minute, parseOpsMetricsCollectorInterval("30"))
	require.Equal(t, 5*time.Minute, parseOpsMetricsCollectorInterval("300"))
	require.Equal(t, time.Hour, parseOpsMetricsCollectorInterval("7200"))
}

func TestResolveOpsRedisHealthRequiresConsecutiveFreshFailures(t *testing.T) {
	now := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	interval := time.Minute

	t.Run("success is immediately healthy", func(t *testing.T) {
		got := resolveOpsRedisHealth(true, nil, now, interval)
		require.NotNil(t, got)
		require.True(t, *got)
	})

	t.Run("first failure is unknown", func(t *testing.T) {
		got := resolveOpsRedisHealth(false, &OpsSystemMetricsSnapshot{
			CreatedAt: now.Add(-time.Minute),
			RedisOK:   boolPtr(true),
		}, now, interval)
		require.Nil(t, got)
	})

	t.Run("second consecutive failure is unhealthy", func(t *testing.T) {
		got := resolveOpsRedisHealth(false, &OpsSystemMetricsSnapshot{
			CreatedAt: now.Add(-time.Minute),
			RedisOK:   nil,
		}, now, interval)
		require.NotNil(t, got)
		require.False(t, *got)
	})

	t.Run("stale failure history is ignored", func(t *testing.T) {
		got := resolveOpsRedisHealth(false, &OpsSystemMetricsSnapshot{
			CreatedAt: now.Add(-3 * time.Minute),
			RedisOK:   boolPtr(false),
		}, now, interval)
		require.Nil(t, got)
	})
}

func TestNormalizeOpsRedisPoolStats(t *testing.T) {
	total, idle := normalizeOpsRedisPoolStats(1, 218)
	require.Equal(t, 1, total)
	require.Equal(t, 1, idle)

	total, idle = normalizeOpsRedisPoolStats(-1, -2)
	require.Zero(t, total)
	require.Zero(t, idle)
}

func TestNormalizeOpsRedisSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)

	t.Run("stale snapshot does not report current health", func(t *testing.T) {
		metrics := &OpsSystemMetricsSnapshot{
			CreatedAt:      now.Add(-3 * time.Minute),
			RedisOK:        boolPtr(false),
			RedisConnTotal: intPtr(1),
			RedisConnIdle:  intPtr(218),
		}
		normalizeOpsRedisSnapshot(metrics, now, time.Minute)
		require.Nil(t, metrics.RedisOK)
		require.Nil(t, metrics.RedisConnTotal)
		require.Nil(t, metrics.RedisConnIdle)
	})

	t.Run("fresh snapshot clamps prewarming counters", func(t *testing.T) {
		metrics := &OpsSystemMetricsSnapshot{
			CreatedAt:      now.Add(-time.Minute),
			RedisOK:        boolPtr(true),
			RedisConnTotal: intPtr(1),
			RedisConnIdle:  intPtr(218),
		}
		normalizeOpsRedisSnapshot(metrics, now, time.Minute)
		require.Equal(t, 1, *metrics.RedisConnTotal)
		require.Equal(t, 1, *metrics.RedisConnIdle)
	})
}
