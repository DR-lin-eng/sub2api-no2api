package repository

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBuildRedisOptions(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host:                "localhost",
			Port:                6379,
			Username:            "app-user",
			Password:            "secret",
			DB:                  2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 4,
			PoolSize:            100,
			MinIdleConns:        10,
			MaxIdleConns:        25,
		},
	}

	opts := buildRedisOptions(cfg)
	require.Equal(t, "localhost:6379", opts.Addr)
	require.Equal(t, "app-user", opts.Username)
	require.Equal(t, "secret", opts.Password)
	require.Equal(t, 2, opts.DB)
	require.Equal(t, 5*time.Second, opts.DialTimeout)
	require.Equal(t, 3*time.Second, opts.ReadTimeout)
	require.Equal(t, 4*time.Second, opts.WriteTimeout)
	require.Equal(t, 100, opts.PoolSize)
	require.Equal(t, 10, opts.MinIdleConns)
	require.Equal(t, 25, opts.MaxIdleConns)
	require.Nil(t, opts.TLSConfig)

	// Test case with TLS enabled
	cfgTLS := &config.Config{
		Redis: config.RedisConfig{
			Host:      "localhost",
			EnableTLS: true,
		},
	}
	optsTLS := buildRedisOptions(cfgTLS)
	require.NotNil(t, optsTLS.TLSConfig)
	require.Equal(t, "localhost", optsTLS.TLSConfig.ServerName)
}

func TestClampRedisMaxIdleConns(t *testing.T) {
	tests := []struct {
		name         string
		poolSize     int
		minIdleConns int
		maxIdleConns int
		want         int
	}{
		{name: "explicit limit", poolSize: 100, minIdleConns: 10, maxIdleConns: 25, want: 25},
		{name: "limited by pool size", poolSize: 64, minIdleConns: 10, maxIdleConns: 256, want: 64},
		{name: "raised to minimum idle", poolSize: 100, minIdleConns: 40, maxIdleConns: 20, want: 40},
		{name: "zero disables limit", poolSize: 100, minIdleConns: 10, maxIdleConns: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, clampRedisMaxIdleConns(tt.poolSize, tt.minIdleConns, tt.maxIdleConns))
		})
	}
}

func TestRedisMaxIdleConnsReleasesConnectionsAfterBurst(t *testing.T) {
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	host, portText, err := net.SplitHostPort(server.Addr())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	const (
		poolSize     = 16
		maxIdleConns = 3
	)
	client := InitRedis(&config.Config{Redis: config.RedisConfig{
		Host:                host,
		Port:                port,
		DialTimeoutSeconds:  1,
		ReadTimeoutSeconds:  1,
		WriteTimeoutSeconds: 1,
		PoolSize:            poolSize,
		MaxIdleConns:        maxIdleConns,
	}})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	ctx := context.Background()
	connections := make([]*redis.Conn, 0, poolSize)
	for range poolSize {
		conn := client.Conn()
		require.NoError(t, conn.Ping(ctx).Err())
		connections = append(connections, conn)
	}
	require.Equal(t, uint32(poolSize), client.PoolStats().TotalConns)
	require.Zero(t, client.PoolStats().IdleConns)

	for _, conn := range connections {
		require.NoError(t, conn.Close())
	}
	require.Equal(t, uint32(maxIdleConns), client.PoolStats().TotalConns)
	require.Equal(t, uint32(maxIdleConns), client.PoolStats().IdleConns)
}
