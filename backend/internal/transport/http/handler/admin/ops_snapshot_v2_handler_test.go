package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

type opsSnapshotQueryProbe struct {
	service.OpsRepository

	overview          atomic.Int64
	throughputTrend   atomic.Int64
	latencyHistogram  atomic.Int64
	errorTrend        atomic.Int64
	errorDistribution atomic.Int64

	reuseErrorTrendCounts atomic.Bool
	excludeSwitchCounts   atomic.Bool
	throughputResponse    *service.OpsThroughputTrendResponse
	errorTrendResponse    *service.OpsErrorTrendResponse
	overviewDelay         time.Duration
}

func (p *opsSnapshotQueryProbe) GetDashboardOverview(context.Context, *service.OpsDashboardFilter) (*service.OpsDashboardOverview, error) {
	p.overview.Add(1)
	if p.overviewDelay > 0 {
		time.Sleep(p.overviewDelay)
	}
	return &service.OpsDashboardOverview{}, nil
}

func (p *opsSnapshotQueryProbe) GetThroughputTrend(_ context.Context, filter *service.OpsDashboardFilter, _ int) (*service.OpsThroughputTrendResponse, error) {
	p.throughputTrend.Add(1)
	if filter != nil {
		p.reuseErrorTrendCounts.Store(filter.ReuseErrorTrendCounts)
		p.excludeSwitchCounts.Store(filter.ExcludeSwitchCounts)
	}
	if p.throughputResponse != nil {
		return p.throughputResponse, nil
	}
	return &service.OpsThroughputTrendResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetLatencyHistogram(context.Context, *service.OpsDashboardFilter) (*service.OpsLatencyHistogramResponse, error) {
	p.latencyHistogram.Add(1)
	return &service.OpsLatencyHistogramResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetErrorTrend(context.Context, *service.OpsDashboardFilter, int) (*service.OpsErrorTrendResponse, error) {
	p.errorTrend.Add(1)
	if p.errorTrendResponse != nil {
		return p.errorTrendResponse, nil
	}
	return &service.OpsErrorTrendResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetErrorDistribution(context.Context, *service.OpsDashboardFilter) (*service.OpsErrorDistributionResponse, error) {
	p.errorDistribution.Add(1)
	return &service.OpsErrorDistributionResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetLatestSystemMetrics(context.Context, int) (*service.OpsSystemMetricsSnapshot, error) {
	return nil, sql.ErrNoRows
}

func (p *opsSnapshotQueryProbe) ListJobHeartbeats(context.Context) ([]*service.OpsJobHeartbeat, error) {
	return nil, nil
}

func TestGetDashboardSnapshotV2SkipsDisabledPanelQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

	probe := &opsSnapshotQueryProbe{}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/snapshot-v2", NewOpsHandler(svc).GetDashboardSnapshotV2)

	url := "/admin/ops/dashboard/snapshot-v2?start_time=2026-07-16T00:00:00Z&end_time=2026-07-16T01:00:00Z" +
		"&include_throughput_trend=false&include_latency_histogram=false" +
		"&include_error_trend=false&include_error_distribution=false"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := probe.overview.Load(); got != 1 {
		t.Fatalf("overview queries = %d, want 1", got)
	}
	if got := probe.throughputTrend.Load() + probe.latencyHistogram.Load() + probe.errorTrend.Load() + probe.errorDistribution.Load(); got != 0 {
		t.Fatalf("optional panel queries = %d, want 0", got)
	}
	for _, field := range []string{"throughput_trend", "latency_histogram", "error_trend", "error_distribution"} {
		if strings.Contains(rec.Body.String(), `"`+field+`"`) {
			t.Fatalf("disabled field %q unexpectedly present in response: %s", field, rec.Body.String())
		}
	}
}

func TestGetDashboardSnapshotV2IncludesAllPanelsByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

	probe := &opsSnapshotQueryProbe{}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/snapshot-v2", NewOpsHandler(svc).GetDashboardSnapshotV2)

	url := "/admin/ops/dashboard/snapshot-v2?start_time=2026-07-16T02:00:00Z&end_time=2026-07-16T03:00:00Z"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if probe.overview.Load() != 1 || probe.throughputTrend.Load() != 1 || probe.latencyHistogram.Load() != 1 || probe.errorTrend.Load() != 1 || probe.errorDistribution.Load() != 1 {
		t.Fatalf(
			"query counts overview=%d throughput=%d latency=%d error_trend=%d error_distribution=%d, want all 1",
			probe.overview.Load(),
			probe.throughputTrend.Load(),
			probe.latencyHistogram.Load(),
			probe.errorTrend.Load(),
			probe.errorDistribution.Load(),
		)
	}
}

func TestGetDashboardSnapshotV2ReusesErrorTrendForThroughput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

	bucket := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	probe := &opsSnapshotQueryProbe{
		throughputResponse: &service.OpsThroughputTrendResponse{
			Bucket: "1m",
			Points: []*service.OpsThroughputTrendPoint{{
				BucketStart:   bucket,
				RequestCount:  7,
				TokenConsumed: 120,
				QPS:           0.1,
				TPS:           2,
			}},
		},
		errorTrendResponse: &service.OpsErrorTrendResponse{
			Bucket: "1m",
			Points: []*service.OpsErrorTrendPoint{{
				BucketStart:     bucket,
				ErrorCountTotal: 3,
			}},
		},
	}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/snapshot-v2", NewOpsHandler(svc).GetDashboardSnapshotV2)

	url := "/admin/ops/dashboard/snapshot-v2?start_time=2026-07-16T04:00:00Z&end_time=2026-07-16T04:01:00Z" +
		"&include_throughput_trend=true&include_error_trend=true" +
		"&include_latency_histogram=false&include_error_distribution=false"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !probe.reuseErrorTrendCounts.Load() {
		t.Fatal("throughput query did not reuse error trend counts")
	}
	if !probe.excludeSwitchCounts.Load() {
		t.Fatal("snapshot calculated switch counts by default")
	}

	var body struct {
		Data opsDashboardSnapshotV2Response `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ThroughputTrend == nil || len(body.Data.ThroughputTrend.Points) != 1 {
		t.Fatalf("unexpected throughput response: %+v", body.Data.ThroughputTrend)
	}
	point := body.Data.ThroughputTrend.Points[0]
	if point.RequestCount != 10 || point.QPS != 0.2 {
		t.Fatalf("merged throughput point = %+v, want request_count=10 qps=0.2", point)
	}
}

func TestGetDashboardSnapshotV2CanIncludeSwitchCountsExplicitly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

	probe := &opsSnapshotQueryProbe{}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/snapshot-v2", NewOpsHandler(svc).GetDashboardSnapshotV2)

	url := "/admin/ops/dashboard/snapshot-v2?start_time=2026-07-16T05:00:00Z&end_time=2026-07-16T05:01:00Z" +
		"&include_switch_count=true&include_latency_histogram=false&include_error_distribution=false"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if probe.excludeSwitchCounts.Load() {
		t.Fatal("explicit include_switch_count=true was ignored")
	}
}

func TestGetDashboardSnapshotV2CoalescesRelativeWindowMisses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
	probe := &opsSnapshotQueryProbe{overviewDelay: 40 * time.Millisecond}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/snapshot-v2", NewOpsHandler(svc).GetDashboardSnapshotV2)
	url := "/admin/ops/dashboard/snapshot-v2?time_range=1h" +
		"&include_throughput_trend=false&include_error_trend=false" +
		"&include_latency_histogram=false&include_error_distribution=false"

	const clients = 12
	var wg sync.WaitGroup
	wg.Add(clients)
	statuses := make(chan int, clients)
	for range clients {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
			statuses <- rec.Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
	}
	if got := probe.overview.Load(); got != 1 {
		t.Fatalf("overview queries = %d, want 1", got)
	}
}
