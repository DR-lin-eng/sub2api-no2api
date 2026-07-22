package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type upstreamBillingRatesResponse struct {
	Items    []service.UpstreamBillingRateSnapshotItem `json:"items"`
	Total    int64                                     `json:"total"`
	Page     int                                       `json:"page"`
	PageSize int                                       `json:"page_size"`
}

type upstreamBillingRateSnapshotService interface {
	ListUpstreamBillingRateSnapshots(
		context.Context,
		int,
		int,
		service.UpstreamBillingRateListFilters,
		string,
		string,
	) ([]service.UpstreamBillingRateSnapshotItem, int64, error)
}

// GetUpstreamBillingRates returns only the persisted snapshot projection for
// the current page. It never contacts an upstream.
func (h *AccountHandler) GetUpstreamBillingRates(c *gin.Context) {
	snapshotService, ok := h.adminService.(upstreamBillingRateSnapshotService)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "account service unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	filters := service.UpstreamBillingRateListFilters{
		Platform: c.Query("platform"), AccountType: c.Query("type"), Status: c.Query("status"),
		Search: search, PrivacyMode: strings.TrimSpace(c.Query("privacy_mode")),
	}
	if groupQuery := c.Query("group"); groupQuery != "" {
		if groupQuery == accountListGroupUngroupedQueryValue {
			filters.GroupID = service.AccountListGroupUngrouped
		} else {
			parsed, err := strconv.ParseInt(groupQuery, 10, 64)
			if err != nil || parsed < 0 {
				response.BadRequest(c, "invalid group filter")
				return
			}
			filters.GroupID = parsed
		}
	}
	items, total, err := snapshotService.ListUpstreamBillingRateSnapshots(
		c.Request.Context(), page, pageSize, filters,
		c.DefaultQuery("sort_by", "name"), c.DefaultQuery("sort_order", "asc"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload := upstreamBillingRatesResponse{Items: items, Total: total, Page: page, PageSize: pageSize}
	c.Header("Cache-Control", "private, no-cache")
	etag := buildUpstreamBillingRatesETag(payload)
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	response.Success(c, payload)
}

func buildUpstreamBillingRatesETag(payload upstreamBillingRatesResponse) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}
