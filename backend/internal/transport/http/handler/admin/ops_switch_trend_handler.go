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

var opsSwitchTrendCache = newSnapshotCache(60 * time.Second)

type opsSwitchTrendCacheKey struct {
	StartTime    string               `json:"start_time"`
	EndTime      string               `json:"end_time"`
	Platform     string               `json:"platform"`
	GroupID      *int64               `json:"group_id"`
	QueryMode    service.OpsQueryMode `json:"mode"`
	BucketSecond int                  `json:"bucket_second"`
}

// GetDashboardSwitchTrend isolates the expensive upstream_errors expansion
// from the core snapshot and coalesces concurrent admin refreshes.
func (h *OpsHandler) GetDashboardSwitchTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	startTime, endTime, err := parseOpsTimeRange(c, "5h")
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
	if value := strings.TrimSpace(c.Query("group_id")); value != "" {
		id, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	cacheStart, cacheEnd := opsDashboardCacheWindowKey(c, startTime, endTime, "5h")
	keyRaw, _ := json.Marshal(opsSwitchTrendCacheKey{
		StartTime:    cacheStart,
		EndTime:      cacheEnd,
		Platform:     filter.Platform,
		GroupID:      filter.GroupID,
		QueryMode:    filter.QueryMode,
		BucketSecond: bucketSeconds,
	})
	entry, hit, err := opsSwitchTrendCache.GetOrLoad(string(keyRaw), func() (any, error) {
		f := *filter
		return h.opsService.GetSwitchTrend(context.WithoutCancel(c.Request.Context()), &f, bucketSeconds)
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if entry.ETag != "" {
		c.Header("ETag", entry.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), entry.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, entry.Payload)
}
