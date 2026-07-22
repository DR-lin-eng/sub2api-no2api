package admin

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
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
}

func (p *opsSnapshotQueryProbe) GetDashboardOverview(context.Context, *service.OpsDashboardFilter) (*service.OpsDashboardOverview, error) {
	p.overview.Add(1)
	return &service.OpsDashboardOverview{}, nil
}

func (p *opsSnapshotQueryProbe) GetThroughputTrend(context.Context, *service.OpsDashboardFilter, int) (*service.OpsThroughputTrendResponse, error) {
	p.throughputTrend.Add(1)
	return &service.OpsThroughputTrendResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetLatencyHistogram(context.Context, *service.OpsDashboardFilter) (*service.OpsLatencyHistogramResponse, error) {
	p.latencyHistogram.Add(1)
	return &service.OpsLatencyHistogramResponse{}, nil
}

func (p *opsSnapshotQueryProbe) GetErrorTrend(context.Context, *service.OpsDashboardFilter, int) (*service.OpsErrorTrendResponse, error) {
	p.errorTrend.Add(1)
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
