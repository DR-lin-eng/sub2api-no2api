package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

func ClusterReadiness(releaseService *service.ClusterReleaseService) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/ready" ||
			path == "/api/v1/admin/cluster" || strings.HasPrefix(path, "/api/v1/admin/cluster/") {
			c.Next()
			return
		}
		if releaseService == nil || releaseService.TryBeginRequest() {
			if releaseService != nil {
				defer releaseService.EndRequest()
			}
			c.Next()
			return
		}
		readiness := releaseService.GetReadiness()
		metadata := map[string]string{
			"node_id":         readiness.NodeID,
			"node_name":       readiness.NodeName,
			"current_version": readiness.CurrentVersion,
		}
		if readiness.DesiredVersion != "" {
			metadata["desired_version"] = readiness.DesiredVersion
		}
		if readiness.RolloutID != "" {
			metadata["rollout_id"] = readiness.RolloutID
		}
		c.Abort()
		response.ErrorWithDetails(
			c,
			http.StatusServiceUnavailable,
			"node is not ready to accept traffic",
			"NODE_NOT_READY",
			metadata,
		)
	}
}
