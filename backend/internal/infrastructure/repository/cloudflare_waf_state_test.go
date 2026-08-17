package repository

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newCloudflareWAFStateStoreForTest(t *testing.T) *cloudflareWAFStateStore {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return newCloudflareWAFStateStore(client)
}

func TestCloudflareWAFStateStoreExtendsAndPrunesBlocks(t *testing.T) {
	store := newCloudflareWAFStateStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	firstExpiry := now.Add(time.Minute)
	secondExpiry := now.Add(2 * time.Minute)
	require.NoError(t, store.UpsertBlocks(t.Context(), []cloudflareBlockRequest{{value: "203.0.113.10", expiresAt: firstExpiry}}))
	require.NoError(t, store.UpsertBlocks(t.Context(), []cloudflareBlockRequest{{value: "203.0.113.10", expiresAt: now.Add(30 * time.Second)}}))
	require.NoError(t, store.UpsertBlocks(t.Context(), []cloudflareBlockRequest{{value: "203.0.113.10", expiresAt: secondExpiry}}))

	entries, removed, total, err := store.Snapshot(t.Context(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.Equal(t, int64(1), total)
	require.Len(t, entries, 1)
	require.Equal(t, secondExpiry, entries[0].ExpiresAt)

	entries, removed, total, err = store.Snapshot(t.Context(), secondExpiry.Add(time.Millisecond), 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	require.Zero(t, total)
	require.Empty(t, entries)
}

func TestCloudflareWAFStateStoreCachesSyncAndAnalytics(t *testing.T) {
	store := newCloudflareWAFStateStoreForTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	syncState := cloudflareWAFSyncState{
		Revision: "revision", Binding: "binding", SyncedEntries: 12, SyncedAt: now,
	}
	require.NoError(t, store.SaveSyncState(t.Context(), syncState))
	loadedSync, err := store.LoadSyncState(t.Context())
	require.NoError(t, err)
	require.Equal(t, syncState, loadedSync)

	analytics := cloudflareWAFAnalyticsSnapshot{
		Binding: "binding", Hostname: "api.example.com", Hostnames: []string{"api.example.com", "edge.example.com"},
		HostnameStats: []cloudflareWAFHostnameAnalyticsSnapshot{
			{Hostname: "api.example.com", Requests24h: 800, BlockedRequests24h: 20},
			{Hostname: "edge.example.com", Requests24h: 200, BlockedRequests24h: 5},
		}, HostnameRequests24h: 1000,
		BlockedRequests24h: 25, WindowStart: now.Add(-24 * time.Hour), UpdatedAt: now, CheckedAt: now,
	}
	require.NoError(t, store.SaveAnalytics(t.Context(), analytics, time.Hour))
	loadedAnalytics, err := store.LoadAnalytics(t.Context())
	require.NoError(t, err)
	require.Equal(t, analytics, loadedAnalytics)
}
