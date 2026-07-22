package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSStateStore_BindGetDeleteResponseAccount(t *testing.T) {
	cache := &stubGatewayCache{}
	store := NewOpenAIWSStateStore(cache)
	ctx := context.Background()
	groupID := int64(7)

	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_abc", 101, time.Minute))

	accountID, err := store.GetResponseAccount(ctx, groupID, "resp_abc")
	require.NoError(t, err)
	require.Equal(t, int64(101), accountID)

	require.NoError(t, store.DeleteResponseAccount(ctx, groupID, "resp_abc"))
	accountID, err = store.GetResponseAccount(ctx, groupID, "resp_abc")
	require.NoError(t, err)
	require.Zero(t, accountID)
}

func TestOpenAIWSStateStore_ResponseConnTTL(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	store.BindResponseConn("resp_conn", "conn_1", 30*time.Millisecond)

	connID, ok := store.GetResponseConn("resp_conn")
	require.True(t, ok)
	require.Equal(t, "conn_1", connID)

	time.Sleep(60 * time.Millisecond)
	_, ok = store.GetResponseConn("resp_conn")
	require.False(t, ok)
}

func TestOpenAIWSStateStore_SessionTurnStateTTL(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	ctx := context.Background()
	require.NoError(t, store.BindSessionTurnState(ctx, 9, "session_hash_1", "turn_state_1", 30*time.Millisecond))

	state, ok, err := store.GetSessionTurnState(ctx, 9, "session_hash_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "turn_state_1", state)

	// group 隔离
	_, ok, err = store.GetSessionTurnState(ctx, 10, "session_hash_1")
	require.NoError(t, err)
	require.False(t, ok)

	time.Sleep(60 * time.Millisecond)
	_, ok, err = store.GetSessionTurnState(ctx, 9, "session_hash_1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestOpenAIWSStateStore_SessionConnTTL(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	store.BindSessionConn(9, "session_hash_conn_1", "conn_1", 30*time.Millisecond)

	connID, ok := store.GetSessionConn(9, "session_hash_conn_1")
	require.True(t, ok)
	require.Equal(t, "conn_1", connID)

	// group 隔离
	_, ok = store.GetSessionConn(10, "session_hash_conn_1")
	require.False(t, ok)

	time.Sleep(60 * time.Millisecond)
	_, ok = store.GetSessionConn(9, "session_hash_conn_1")
	require.False(t, ok)
}

func TestOpenAIWSStateStore_GetResponseAccount_NoStaleAfterCacheMiss(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	store := NewOpenAIWSStateStore(cache)
	ctx := context.Background()
	groupID := int64(17)
	responseID := "resp_cache_stale"
	cacheKey := openAIWSResponseAccountCacheKey(responseID)

	cache.sessionBindings[cacheKey] = 501
	accountID, err := store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.Equal(t, int64(501), accountID)

	delete(cache.sessionBindings, cacheKey)
	accountID, err = store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.Zero(t, accountID, "上游缓存失效后不应继续命中本地陈旧映射")
}

type openAIWSSharedStateValue struct {
	value     string
	expiresAt time.Time
}

type stubOpenAIWSSharedCache struct {
	stubGatewayCache

	mu        sync.Mutex
	states    map[string]openAIWSSharedStateValue
	setErr    error
	getErr    error
	deleteErr error
}

func (c *stubOpenAIWSSharedCache) SetOpenAIWSState(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	if c.states == nil {
		c.states = make(map[string]openAIWSSharedStateValue)
	}
	c.states[key] = openAIWSSharedStateValue{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (c *stubOpenAIWSSharedCache) GetOpenAIWSState(_ context.Context, key string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return "", false, c.getErr
	}
	value, ok := c.states[key]
	if !ok || time.Now().After(value.expiresAt) {
		delete(c.states, key)
		return "", false, nil
	}
	return value.value, true, nil
}

func (c *stubOpenAIWSSharedCache) DeleteOpenAIWSState(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleteErr != nil {
		return c.deleteErr
	}
	delete(c.states, key)
	return nil
}

func TestOpenAIWSStateStore_SharedStateCrossNodeInvalidation(t *testing.T) {
	cache := &stubOpenAIWSSharedCache{}
	storeA := NewOpenAIWSStateStore(cache)
	storeB := NewOpenAIWSStateStore(cache)
	ctx := context.Background()

	require.NoError(t, storeA.BindResponseAccount(ctx, 7, "resp_cross_node", 101, time.Minute))
	accountID, err := storeB.GetResponseAccount(ctx, 7, "resp_cross_node")
	require.NoError(t, err)
	require.Equal(t, int64(101), accountID)
	require.NoError(t, storeB.DeleteResponseAccount(ctx, 7, "resp_cross_node"))
	accountID, err = storeA.GetResponseAccount(ctx, 7, "resp_cross_node")
	require.NoError(t, err)
	require.Zero(t, accountID, "节点 B 删除 Redis 映射后，节点 A 不应命中本地旧值")

	require.NoError(t, storeA.BindSessionTurnState(ctx, 7, "session_cross_node", "turn_cross_node", time.Minute))
	turnState, ok, err := storeB.GetSessionTurnState(ctx, 7, "session_cross_node")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "turn_cross_node", turnState)
	require.NoError(t, storeB.DeleteSessionTurnState(ctx, 7, "session_cross_node"))
	_, ok, err = storeA.GetSessionTurnState(ctx, 7, "session_cross_node")
	require.NoError(t, err)
	require.False(t, ok, "节点 B 删除 turn state 后，节点 A 应立即失效")

	storeA.BindResponseConn("resp_local", "conn_a", time.Minute)
	storeA.BindSessionConn(7, "session_local", "conn_a", time.Minute)
	_, ok = storeB.GetResponseConn("resp_local")
	require.False(t, ok, "response conn ID 只能在连接所属节点可见")
	_, ok = storeB.GetSessionConn(7, "session_local")
	require.False(t, ok, "session conn ID 只能在连接所属节点可见")
}

func TestOpenAIWSStateStore_SharedStateFailureDoesNotFallBackToLocal(t *testing.T) {
	cache := &stubOpenAIWSSharedCache{setErr: errors.New("redis unavailable")}
	store := NewOpenAIWSStateStore(cache)
	ctx := context.Background()

	err := store.BindSessionTurnState(ctx, 3, "session_redis_failure", "turn_state", time.Minute)
	require.ErrorContains(t, err, "redis unavailable")

	turnState, ok, err := store.GetSessionTurnState(ctx, 3, "session_redis_failure")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, turnState, "Redis 配置存在时不得回退到可能陈旧的本地副本")
}

func TestOpenAIWSStateStore_MaybeCleanupRemovesExpiredIncrementally(t *testing.T) {
	raw := NewOpenAIWSStateStore(nil)
	store, ok := raw.(*defaultOpenAIWSStateStore)
	require.True(t, ok)

	expiredAt := time.Now().Add(-time.Minute)
	total := 2048
	store.responseToConnMu.Lock()
	for i := 0; i < total; i++ {
		store.responseToConn[fmt.Sprintf("resp_%d", i)] = openAIWSConnBinding{
			connID:    "conn_incremental",
			expiresAt: expiredAt,
		}
	}
	store.responseToConnMu.Unlock()

	store.lastCleanupUnixNano.Store(time.Now().Add(-2 * openAIWSStateStoreCleanupInterval).UnixNano())
	store.maybeCleanup()

	store.responseToConnMu.RLock()
	remainingAfterFirst := len(store.responseToConn)
	store.responseToConnMu.RUnlock()
	require.Less(t, remainingAfterFirst, total, "单轮 cleanup 应至少有进展")
	require.Greater(t, remainingAfterFirst, 0, "增量清理不要求单轮清空全部键")

	for i := 0; i < 8; i++ {
		store.lastCleanupUnixNano.Store(time.Now().Add(-2 * openAIWSStateStoreCleanupInterval).UnixNano())
		store.maybeCleanup()
	}

	store.responseToConnMu.RLock()
	remaining := len(store.responseToConn)
	store.responseToConnMu.RUnlock()
	require.Zero(t, remaining, "多轮 cleanup 后应逐步清空全部过期键")
}

func TestEnsureBindingCapacity_EvictsOneWhenMapIsFull(t *testing.T) {
	bindings := map[string]int{
		"a": 1,
		"b": 2,
	}

	ensureBindingCapacity(bindings, "c", 2)
	bindings["c"] = 3

	require.Len(t, bindings, 2)
	require.Equal(t, 3, bindings["c"])
}

func TestEnsureBindingCapacity_DoesNotEvictWhenUpdatingExistingKey(t *testing.T) {
	bindings := map[string]int{
		"a": 1,
		"b": 2,
	}

	ensureBindingCapacity(bindings, "a", 2)
	bindings["a"] = 9

	require.Len(t, bindings, 2)
	require.Equal(t, 9, bindings["a"])
}

type openAIWSStateStoreTimeoutProbeCache struct {
	setHasDeadline    bool
	getHasDeadline    bool
	deleteHasDeadline bool
	setDeadlineDelta  time.Duration
	getDeadlineDelta  time.Duration
	delDeadlineDelta  time.Duration
	sharedSetDeadline time.Duration
	sharedGetDeadline time.Duration
	sharedDelDeadline time.Duration
}

func (c *openAIWSStateStoreTimeoutProbeCache) GetSessionAccountID(ctx context.Context, _ int64, _ string) (int64, error) {
	if deadline, ok := ctx.Deadline(); ok {
		c.getHasDeadline = true
		c.getDeadlineDelta = time.Until(deadline)
	}
	return 123, nil
}

func (c *openAIWSStateStoreTimeoutProbeCache) SetSessionAccountID(ctx context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok {
		c.setHasDeadline = true
		c.setDeadlineDelta = time.Until(deadline)
	}
	return errors.New("set failed")
}

func (c *openAIWSStateStoreTimeoutProbeCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *openAIWSStateStoreTimeoutProbeCache) DeleteSessionAccountID(ctx context.Context, _ int64, _ string) error {
	if deadline, ok := ctx.Deadline(); ok {
		c.deleteHasDeadline = true
		c.delDeadlineDelta = time.Until(deadline)
	}
	return nil
}

func (c *openAIWSStateStoreTimeoutProbeCache) SetOpenAIWSState(ctx context.Context, _, _ string, _ time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok {
		c.sharedSetDeadline = time.Until(deadline)
	}
	return errors.New("shared set failed")
}

func (c *openAIWSStateStoreTimeoutProbeCache) GetOpenAIWSState(ctx context.Context, _ string) (string, bool, error) {
	if deadline, ok := ctx.Deadline(); ok {
		c.sharedGetDeadline = time.Until(deadline)
	}
	return "turn_from_redis", true, nil
}

func (c *openAIWSStateStoreTimeoutProbeCache) DeleteOpenAIWSState(ctx context.Context, _ string) error {
	if deadline, ok := ctx.Deadline(); ok {
		c.sharedDelDeadline = time.Until(deadline)
	}
	return nil
}

func TestOpenAIWSStateStore_RedisOpsUseShortTimeout(t *testing.T) {
	probe := &openAIWSStateStoreTimeoutProbeCache{}
	store := NewOpenAIWSStateStore(probe)
	ctx := context.Background()
	groupID := int64(5)

	err := store.BindResponseAccount(ctx, groupID, "resp_timeout_probe", 11, time.Minute)
	require.Error(t, err)

	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_timeout_probe")
	require.NoError(t, getErr)
	require.Equal(t, int64(123), accountID, "Redis 配置存在时应始终从权威缓存读取")

	require.NoError(t, store.DeleteResponseAccount(ctx, groupID, "resp_timeout_probe"))
	require.Error(t, store.BindSessionTurnState(ctx, groupID, "session_timeout_probe", "turn", time.Minute))
	turnState, ok, turnErr := store.GetSessionTurnState(ctx, groupID, "session_timeout_probe")
	require.NoError(t, turnErr)
	require.True(t, ok)
	require.Equal(t, "turn_from_redis", turnState)
	require.NoError(t, store.DeleteSessionTurnState(ctx, groupID, "session_timeout_probe"))

	require.True(t, probe.setHasDeadline, "SetSessionAccountID 应携带独立超时上下文")
	require.True(t, probe.deleteHasDeadline, "DeleteSessionAccountID 应携带独立超时上下文")
	require.True(t, probe.getHasDeadline, "GetSessionAccountID 应从 Redis 权威读取")
	require.Greater(t, probe.setDeadlineDelta, 2*time.Second)
	require.LessOrEqual(t, probe.setDeadlineDelta, 3*time.Second)
	require.Greater(t, probe.delDeadlineDelta, 2*time.Second)
	require.LessOrEqual(t, probe.delDeadlineDelta, 3*time.Second)
	require.Greater(t, probe.sharedSetDeadline, 2*time.Second)
	require.LessOrEqual(t, probe.sharedSetDeadline, 3*time.Second)
	require.Greater(t, probe.sharedGetDeadline, 2*time.Second)
	require.LessOrEqual(t, probe.sharedGetDeadline, 3*time.Second)
	require.Greater(t, probe.sharedDelDeadline, 2*time.Second)
	require.LessOrEqual(t, probe.sharedDelDeadline, 3*time.Second)

	probe2 := &openAIWSStateStoreTimeoutProbeCache{}
	store2 := NewOpenAIWSStateStore(probe2)
	accountID2, err2 := store2.GetResponseAccount(ctx, groupID, "resp_cache_only")
	require.NoError(t, err2)
	require.Equal(t, int64(123), accountID2)
	require.True(t, probe2.getHasDeadline, "GetSessionAccountID 在缓存未命中时应携带独立超时上下文")
	require.Greater(t, probe2.getDeadlineDelta, 2*time.Second)
	require.LessOrEqual(t, probe2.getDeadlineDelta, 3*time.Second)
}

func TestWithOpenAIWSStateStoreRedisTimeout_WithParentContext(t *testing.T) {
	ctx, cancel := withOpenAIWSStateStoreRedisTimeout(context.Background())
	defer cancel()
	require.NotNil(t, ctx)
	_, ok := ctx.Deadline()
	require.True(t, ok, "应附加短超时")
}
