package repository

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRPMCacheIncrementDistillation_IsAtomicAndScoped(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := &RPMCacheImpl{rdb: client}

	first, err := cache.IncrementDistillation(context.Background(), 10, 20)
	require.NoError(t, err)
	second, err := cache.IncrementDistillation(context.Background(), 10, 20)
	require.NoError(t, err)
	otherAccount, err := cache.IncrementDistillation(context.Background(), 10, 21)
	require.NoError(t, err)

	require.Equal(t, int64(1), first)
	require.Equal(t, int64(2), second)
	require.Equal(t, int64(1), otherAccount)
	value, err := mini.Get("distill:req:10:20")
	require.NoError(t, err)
	require.Equal(t, "2", value)
}
