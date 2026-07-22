package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

var opsDashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)

const opsDashboardSnapshotV2LoadTimeout = 10 * time.Second

type opsDashboardSnapshotV2Response struct {
	GeneratedAt string `json:"generated_at"`

	Overview          *service.OpsDashboardOverview         `json:"overview"`
	ThroughputTrend   *service.OpsThroughputTrendResponse   `json:"throughput_trend,omitempty"`
	LatencyHistogram  *service.OpsLatencyHistogramResponse  `json:"latency_histogram,omitempty"`
	ErrorTrend        *service.OpsErrorTrendResponse        `json:"error_trend,omitempty"`
	ErrorDistribution *service.OpsErrorDistributionResponse `json:"error_distribution,omitempty"`
}

type opsDashboardSnapshotV2CacheKey struct {
	StartTime                string               `json:"start_time"`
	EndTime                  string               `json:"end_time"`
	Platform                 string               `json:"platform"`
	GroupID                  *int64               `json:"group_id"`
	QueryMode                service.OpsQueryMode `json:"mode"`
	BucketSecond             int                  `json:"bucket_second"`
	IncludeThroughputTrend   bool                 `json:"include_throughput_trend"`
	IncludeLatencyHistogram  bool                 `json:"include_latency_histogram"`
	IncludeErrorTrend        bool                 `json:"include_error_trend"`
	IncludeErrorDistribution bool                 `json:"include_error_distribution"`
	IncludeSwitchCount       bool                 `json:"include_switch_count"`
}

// GetDashboardSnapshotV2 returns ops dashboard core snapshot in one request.
// GET /api/v1/admin/ops/dashboard/snapshot-v2
func (h *OpsHandler) GetDashboardSnapshotV2(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.ToLower(strings.TrimSpace(c.Query("platform"))),
		QueryMode: parseOpsQueryMode(c),
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	includeThroughputTrend := parseBoolQueryWithDefault(c.Query("include_throughput_trend"), true)
	includeLatencyHistogram := parseBoolQueryWithDefault(c.Query("include_latency_histogram"), true)
	includeErrorTrend := parseBoolQueryWithDefault(c.Query("include_error_trend"), true)
	includeErrorDistribution := parseBoolQueryWithDefault(c.Query("include_error_distribution"), true)
	// The snapshot's throughput panel only displays QPS/TPS. Switch-rate data has
	// its own endpoint and chart, so avoid parsing upstream_errors JSON unless an
	// API caller explicitly asks for the legacy field values.
	includeSwitchCount := parseBoolQueryWithDefault(c.Query("include_switch_count"), false)

	cacheStart, cacheEnd := opsDashboardCacheWindowKey(c, startTime, endTime, "1h")
	keyRaw, _ := json.Marshal(opsDashboardSnapshotV2CacheKey{
		StartTime:                cacheStart,
		EndTime:                  cacheEnd,
		Platform:                 filter.Platform,
		GroupID:                  filter.GroupID,
		QueryMode:                filter.QueryMode,
		BucketSecond:             bucketSeconds,
		IncludeThroughputTrend:   includeThroughputTrend,
		IncludeLatencyHistogram:  includeLatencyHistogram,
		IncludeErrorTrend:        includeErrorTrend,
		IncludeErrorDistribution: includeErrorDistribution,
		IncludeSwitchCount:       includeSwitchCount,
	})
	cacheKey := string(keyRaw)

	cached, hit, err := opsDashboardSnapshotV2Cache.GetOrLoad(cacheKey, func() (any, error) {
		f := *filter
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), opsDashboardSnapshotV2LoadTimeout)
		defer cancel()
		core, err := h.opsService.GetDashboardCoreSnapshot(
			loadCtx,
			&f,
			bucketSeconds,
			includeThroughputTrend,
			includeLatencyHistogram,
			includeErrorTrend,
			includeErrorDistribution,
			includeSwitchCount,
		)
		if err != nil {
			return nil, err
		}
		return &opsDashboardSnapshotV2Response{
			GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
			Overview:          core.Overview,
			ThroughputTrend:   core.ThroughputTrend,
			LatencyHistogram:  core.LatencyHistogram,
			ErrorTrend:        core.ErrorTrend,
			ErrorDistribution: core.ErrorDistribution,
		}, nil
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, cached.Payload)
}

func opsDashboardCacheWindowKey(c *gin.Context, startTime, endTime time.Time, defaultRange string) (string, string) {
	if c != nil && strings.TrimSpace(c.Query("start_time")) == "" && strings.TrimSpace(c.Query("end_time")) == "" {
		timeRange := strings.TrimSpace(c.Query("time_range"))
		if timeRange == "" {
			timeRange = defaultRange
		}
		return "relative:" + timeRange, ""
	}
	return startTime.UTC().Format(time.RFC3339Nano), endTime.UTC().Format(time.RFC3339Nano)
}
