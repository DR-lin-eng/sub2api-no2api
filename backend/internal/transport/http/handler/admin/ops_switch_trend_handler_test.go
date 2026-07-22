package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

type opsSwitchTrendQueryProbe struct {
	service.OpsRepository
	calls        atomic.Int64
	windowNanos  atomic.Int64
	bucketSecond atomic.Int64
	delay        time.Duration
}

func (p *opsSwitchTrendQueryProbe) GetSwitchTrend(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	bucketSeconds int,
) (*service.OpsThroughputTrendResponse, error) {
	p.calls.Add(1)
	p.windowNanos.Store(filter.EndTime.Sub(filter.StartTime).Nanoseconds())
	p.bucketSecond.Store(int64(bucketSeconds))
	if p.delay > 0 {
		timer := time.NewTimer(p.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &service.OpsThroughputTrendResponse{Bucket: "5m"}, nil
}

func TestGetDashboardSwitchTrendCoalescesRelativeWindowMisses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsSwitchTrendCache = newSnapshotCache(time.Minute)
	probe := &opsSwitchTrendQueryProbe{delay: 40 * time.Millisecond}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/switch-trend", NewOpsHandler(svc).GetDashboardSwitchTrend)

	const clients = 12
	start := make(chan struct{})
	statuses := make(chan int, clients)
	var wg sync.WaitGroup
	wg.Add(clients)
	for range clients {
		go func() {
			defer wg.Done()
			<-start
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/admin/ops/dashboard/switch-trend?time_range=5h&mode=raw&platform=openai", nil)
			router.ServeHTTP(recorder, request)
			statuses <- recorder.Code
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)

	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
	}
	if got := probe.calls.Load(); got != 1 {
		t.Fatalf("switch queries = %d, want 1", got)
	}
	if got := time.Duration(probe.windowNanos.Load()); got != 5*time.Hour {
		t.Fatalf("query window = %s, want 5h", got)
	}
	if got := probe.bucketSecond.Load(); got != 300 {
		t.Fatalf("bucket seconds = %d, want 300", got)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/ops/dashboard/switch-trend?time_range=5h&mode=raw&platform=OpenAI", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("cached status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Snapshot-Cache"); got != "hit" {
		t.Fatalf("X-Snapshot-Cache = %q, want hit", got)
	}
	if got := probe.calls.Load(); got != 1 {
		t.Fatalf("switch queries after cache hit = %d, want 1", got)
	}
}

func TestGetDashboardSwitchTrendSharedLoadSurvivesCallerCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsSwitchTrendCache = newSnapshotCache(time.Minute)
	probe := &opsSwitchTrendQueryProbe{delay: 10 * time.Millisecond}
	svc := service.NewOpsService(probe, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/ops/dashboard/switch-trend", NewOpsHandler(svc).GetDashboardSwitchTrend)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/admin/ops/dashboard/switch-trend?time_range=5h&mode=raw", nil).
		WithContext(requestCtx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := probe.calls.Load(); got != 1 {
		t.Fatalf("switch queries = %d, want 1", got)
	}
}
