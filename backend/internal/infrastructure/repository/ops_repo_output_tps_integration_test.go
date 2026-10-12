//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestDashboardOutputTPSUsesExactSamplesAndSharedScan(t *testing.T) {
	ctx := context.Background()
	schema := fmt.Sprintf("ops_tps_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	dsn, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(ctx, `CREATE TABLE usage_logs (
        id BIGSERIAL, created_at TIMESTAMPTZ, duration_ms BIGINT, first_token_ms BIGINT,
        output_tokens BIGINT, image_count BIGINT DEFAULT 0, image_output_tokens BIGINT DEFAULT 0,
        billing_mode TEXT DEFAULT 'token', request_type SMALLINT, group_id BIGINT, account_id BIGINT);
        CREATE TABLE accounts (id BIGINT, platform TEXT); INSERT INTO accounts VALUES (1, 'openai');`)
	require.NoError(t, err)
	repo := &opsRepository{db: db}
	start := time.Now().UTC().Truncate(time.Hour)
	end := start.Add(time.Hour)
	filter := &service.OpsDashboardFilter{StartTime: start, EndTime: end}
	for _, fixture := range []struct {
		output, duration, imageCount, imageTokens int
		mode                                      string
		requestType                               service.RequestType
		firstToken                                *int
	}{
		{10, 1000, 0, 0, "token", service.RequestTypeSync, intPointer(100)},
		{20, 1000, 0, 0, "token", service.RequestTypeStream, intPointer(100)},
		{40, 1000, 0, 0, "token", service.RequestTypeWSV2, intPointer(100)},
		{60, 1000, 0, 0, "token", service.RequestTypeCyberBlocked, nil},
		{999, 1000, 1, 0, "token", service.RequestTypeSync, intPointer(100)},
		{999, 1000, 0, 1, "token", service.RequestTypeSync, intPointer(100)},
		{999, 1000, 0, 0, "image", service.RequestTypeSync, intPointer(100)},
		{999, 1000, 0, 0, "live", service.RequestTypeSync, intPointer(100)},
		{0, 1000, 0, 0, "token", service.RequestTypeSync, intPointer(100)},
		{999, 0, 0, 0, "token", service.RequestTypeSync, intPointer(100)},
	} {
		_, err := db.ExecContext(ctx, `INSERT INTO usage_logs
            (created_at, output_tokens, duration_ms, image_count, image_output_tokens, billing_mode, request_type, first_token_ms, group_id, account_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,1)`, start.Add(time.Minute), fixture.output, fixture.duration, fixture.imageCount, fixture.imageTokens, fixture.mode, fixture.requestType, fixture.firstToken)
		require.NoError(t, err)
	}
	_, _, _, raw, err := repo.queryUsageLatencyMetrics(ctx, filter, start, end, true)
	require.NoError(t, err)
	_, preagg, err := repo.queryUsageTTFTAndOutputTPS(ctx, filter, start, end)
	require.NoError(t, err)
	for _, stats := range []*service.OpsOutputTPS{raw, preagg} {
		require.EqualValues(t, 4, stats.SampleCount)
		require.InDelta(t, 11.5, *stats.P5, 0.0001)
		require.InDelta(t, 13, *stats.P10, 0.0001)
		require.InDelta(t, 30, *stats.P50, 0.0001)
		require.InDelta(t, 32.5, *stats.Avg, 0.0001)
	}
	_, empty, err := repo.queryUsageTTFTAndOutputTPS(ctx, filter, end, end.Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, empty.SampleCount)
	require.Nil(t, empty.P50)
	_, err = db.ExecContext(ctx, "TRUNCATE usage_logs")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs
        (created_at, output_tokens, duration_ms, first_token_ms, request_type, group_id, account_id)
        SELECT $1::timestamptz + (i % 3600) * INTERVAL '1 second', 50 + i % 1000, 100 + i % 5000,
            5 + i % 500, $2, 1, 1 FROM generate_series(1,100000) i`, start, service.RequestTypeStream)
	require.NoError(t, err)
	separate := func() {
		_, _, _, err := repo.queryUsageLatency(ctx, filter, start, end)
		require.NoError(t, err)
		join, where, args, _ := buildUsageWhere(filter, start, end, 1)
		var stats service.OpsOutputTPS
		err = db.QueryRowContext(ctx, "SELECT "+opsOutputTPSColumns+" FROM usage_logs ul "+join+" "+where, args...).Scan(&stats.P5, &stats.P10, &stats.P50, &stats.Avg, &stats.SampleCount)
		require.NoError(t, err)
	}
	fused := func() {
		_, _, _, stats, err := repo.queryUsageLatencyMetrics(ctx, filter, start, end, true)
		require.NoError(t, err)
		require.EqualValues(t, 100000, stats.SampleCount)
	}
	median := func(run func()) time.Duration {
		run()
		times := make([]time.Duration, 5)
		for i := range times {
			started := time.Now()
			run()
			times[i] = time.Since(started)
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		return times[len(times)/2]
	}
	t.Logf("100000 rows: separate latency+TPS queries median=%s; fused scan median=%s", median(separate), median(fused))
}

func intPointer(value int) *int { return &value }
