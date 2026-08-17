package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cloudflareWAFBlocksKey    = "sub2api:cloudflare:ingress:waf:blocks"
	cloudflareWAFSyncStateKey = "sub2api:cloudflare:ingress:waf:sync-state"
	cloudflareWAFAnalyticsKey = "sub2api:cloudflare:ingress:waf:analytics"
)

var cloudflareWAFUpsertScript = redis.NewScript(`
for index = 1, #ARGV, 2 do
  local member = ARGV[index]
  local score = tonumber(ARGV[index + 1])
  local current = redis.call('ZSCORE', KEYS[1], member)
  if not current or score > tonumber(current) then
    redis.call('ZADD', KEYS[1], score, member)
  end
end
return #ARGV / 2
`)

type cloudflareWAFStateStore struct {
	rdb *redis.Client
}

type cloudflareWAFStateEntry struct {
	Value     string
	ExpiresAt time.Time
}

type cloudflareWAFSyncState struct {
	Revision      string    `json:"revision"`
	Binding       string    `json:"binding"`
	SyncedEntries int       `json:"synced_entries"`
	SyncedAt      time.Time `json:"synced_at"`
}

type cloudflareWAFAnalyticsSnapshot struct {
	Binding             string                                   `json:"binding"`
	Hostname            string                                   `json:"hostname"`
	Hostnames           []string                                 `json:"hostnames,omitempty"`
	HostnameStats       []cloudflareWAFHostnameAnalyticsSnapshot `json:"hostname_stats,omitempty"`
	HostnameRequests24h uint64                                   `json:"hostname_requests_24h"`
	BlockedRequests24h  uint64                                   `json:"blocked_requests_24h"`
	WindowStart         time.Time                                `json:"window_start"`
	UpdatedAt           time.Time                                `json:"updated_at"`
	CheckedAt           time.Time                                `json:"checked_at"`
	Error               string                                   `json:"error,omitempty"`
}

type cloudflareWAFHostnameAnalyticsSnapshot struct {
	Hostname           string `json:"hostname"`
	Requests24h        uint64 `json:"requests_24h"`
	BlockedRequests24h uint64 `json:"blocked_requests_24h"`
}

func newCloudflareWAFStateStore(rdb *redis.Client) *cloudflareWAFStateStore {
	if rdb == nil {
		return nil
	}
	return &cloudflareWAFStateStore{rdb: rdb}
}

func (s *cloudflareWAFStateStore) UpsertBlocks(ctx context.Context, requests []cloudflareBlockRequest) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	if len(requests) == 0 {
		return nil
	}
	args := make([]any, 0, len(requests)*2)
	for _, request := range requests {
		args = append(args, request.value, request.expiresAt.UnixMilli())
	}
	if _, err := cloudflareWAFUpsertScript.Run(ctx, s.rdb, []string{cloudflareWAFBlocksKey}, args...).Result(); err != nil {
		return fmt.Errorf("persist Cloudflare WAF blocks: %w", err)
	}
	return nil
}

func (s *cloudflareWAFStateStore) Snapshot(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]cloudflareWAFStateEntry, int64, int64, error) {
	if s == nil || s.rdb == nil {
		return nil, 0, 0, fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	if limit < 1 {
		limit = 1
	}
	cutoff := strconv.FormatInt(now.UnixMilli(), 10)
	pipe := s.rdb.TxPipeline()
	removed := pipe.ZRemRangeByScore(ctx, cloudflareWAFBlocksKey, "-inf", cutoff)
	total := pipe.ZCount(ctx, cloudflareWAFBlocksKey, cutoff, "+inf")
	active := pipe.ZRevRangeByScoreWithScores(ctx, cloudflareWAFBlocksKey, &redis.ZRangeBy{
		Min:    cutoff,
		Max:    "+inf",
		Offset: 0,
		Count:  int64(limit),
	})
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, 0, 0, fmt.Errorf("load Cloudflare WAF blocks: %w", err)
	}
	items := active.Val()
	result := make([]cloudflareWAFStateEntry, 0, len(items))
	for _, item := range items {
		value, ok := item.Member.(string)
		if !ok || value == "" {
			continue
		}
		result = append(result, cloudflareWAFStateEntry{
			Value:     value,
			ExpiresAt: time.UnixMilli(int64(item.Score)).UTC(),
		})
	}
	return result, removed.Val(), total.Val(), nil
}

func (s *cloudflareWAFStateStore) CountActive(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.rdb == nil {
		return 0, fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	count, err := s.rdb.ZCount(ctx, cloudflareWAFBlocksKey, strconv.FormatInt(now.UnixMilli(), 10), "+inf").Result()
	if err != nil {
		return 0, fmt.Errorf("count Cloudflare WAF blocks: %w", err)
	}
	return count, nil
}

func (s *cloudflareWAFStateStore) LoadSyncState(ctx context.Context) (cloudflareWAFSyncState, error) {
	var state cloudflareWAFSyncState
	if s == nil || s.rdb == nil {
		return state, fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	raw, err := s.rdb.Get(ctx, cloudflareWAFSyncStateKey).Result()
	if err == redis.Nil {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("load Cloudflare WAF sync state: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return cloudflareWAFSyncState{}, fmt.Errorf("decode Cloudflare WAF sync state: %w", err)
	}
	return state, nil
}

func (s *cloudflareWAFStateStore) SaveSyncState(ctx context.Context, state cloudflareWAFSyncState) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Cloudflare WAF sync state: %w", err)
	}
	if err := s.rdb.Set(ctx, cloudflareWAFSyncStateKey, raw, 0).Err(); err != nil {
		return fmt.Errorf("save Cloudflare WAF sync state: %w", err)
	}
	return nil
}

func (s *cloudflareWAFStateStore) LoadAnalytics(ctx context.Context) (cloudflareWAFAnalyticsSnapshot, error) {
	var snapshot cloudflareWAFAnalyticsSnapshot
	if s == nil || s.rdb == nil {
		return snapshot, fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	raw, err := s.rdb.Get(ctx, cloudflareWAFAnalyticsKey).Result()
	if err == redis.Nil {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, fmt.Errorf("load Cloudflare WAF analytics: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return cloudflareWAFAnalyticsSnapshot{}, fmt.Errorf("decode Cloudflare WAF analytics: %w", err)
	}
	return snapshot, nil
}

func (s *cloudflareWAFStateStore) SaveAnalytics(
	ctx context.Context,
	snapshot cloudflareWAFAnalyticsSnapshot,
	ttl time.Duration,
) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("cloudflare WAF state store is unavailable")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode Cloudflare WAF analytics: %w", err)
	}
	if ttl < time.Hour {
		ttl = time.Hour
	}
	if err := s.rdb.Set(ctx, cloudflareWAFAnalyticsKey, raw, ttl).Err(); err != nil {
		return fmt.Errorf("save Cloudflare WAF analytics: %w", err)
	}
	return nil
}
