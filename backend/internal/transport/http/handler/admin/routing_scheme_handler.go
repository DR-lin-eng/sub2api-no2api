package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

// RoutingSchemeHandler handles admin group routing scheme management.
type RoutingSchemeHandler struct {
	service *service.RoutingSchemeService
}

func NewRoutingSchemeHandler(routingSchemeService *service.RoutingSchemeService) *RoutingSchemeHandler {
	return &RoutingSchemeHandler{service: routingSchemeService}
}

type routingSchemeItemRequest struct {
	GroupID int64                      `json:"group_id" binding:"required"`
	Weight  int                        `json:"weight"`
	Rules   []domain.RoutingSchemeRule `json:"rules"`
}

type routingSchemeRequest struct {
	Name        string                     `json:"name" binding:"required"`
	Description string                     `json:"description"`
	Platform    string                     `json:"platform" binding:"required"`
	Status      string                     `json:"status"`
	SortOrder   int                        `json:"sort_order"`
	SortMode    string                     `json:"sort_mode"`
	Items       []routingSchemeItemRequest `json:"items" binding:"required"`
}

type routingSchemeItemResponse struct {
	GroupID        int64                      `json:"group_id"`
	GroupName      string                     `json:"group_name"`
	Platform       string                     `json:"platform"`
	RateMultiplier float64                    `json:"rate_multiplier"`
	Weight         int                        `json:"weight"`
	Position       int                        `json:"position"`
	Rules          []domain.RoutingSchemeRule `json:"rules"`
	// Read-only runtime metrics; admins cannot edit them.
	Score        float64 `json:"score"`
	P95TTFTMs    float64 `json:"p95_ttft_ms"`
	FailureRate  float64 `json:"failure_rate"`
	FailureCount int64   `json:"failure_count"`
	SampleCount  int64   `json:"sample_count"`
	ColdStart    bool    `json:"cold_start"`
}

type routingSchemeResponse struct {
	ID          int64                       `json:"id"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Platform    string                      `json:"platform"`
	Status      string                      `json:"status"`
	SortOrder   int                         `json:"sort_order"`
	SortMode    string                      `json:"sort_mode"`
	Items       []routingSchemeItemResponse `json:"items"`
	CreatedAt   time.Time                   `json:"created_at"`
	UpdatedAt   time.Time                   `json:"updated_at"`
}

// GetGroupScores returns read-only scores for the groups being edited before a
// scheme is saved. GET /admin/routing-schemes/group-scores?group_ids=1,2
func (h *RoutingSchemeHandler) GetGroupScores(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("group_ids"))
	if raw == "" {
		response.BadRequest(c, "group_ids is required")
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 50 {
		response.BadRequest(c, "too many group_ids")
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "invalid group_ids")
			return
		}
		ids = append(ids, id)
	}
	scores := h.service.GroupScores(c.Request.Context(), ids)
	out := make([]routingGroupScoreResponse, 0, len(ids))
	for _, id := range ids {
		score := scores[id]
		out = append(out, routingGroupScoreResponse{
			GroupID:      id,
			Score:        score.Overall,
			P95TTFTMs:    score.P95TTFTMs,
			FailureRate:  score.FailureRate,
			FailureCount: score.FailureCount,
			SampleCount:  score.SampleCount,
			ColdStart:    score.ColdStart,
		})
	}
	response.Success(c, out)
}

type routingGroupScoreResponse struct {
	GroupID      int64   `json:"group_id"`
	Score        float64 `json:"score"`
	P95TTFTMs    float64 `json:"p95_ttft_ms"`
	FailureRate  float64 `json:"failure_rate"`
	FailureCount int64   `json:"failure_count"`
	SampleCount  int64   `json:"sample_count"`
	ColdStart    bool    `json:"cold_start"`
}

type routingSchemeAnalyticsGroupResponse struct {
	GroupID        int64   `json:"group_id"`
	GroupName      string  `json:"group_name"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`
	Requests       int64   `json:"requests"`
	Successes      int64   `json:"successes"`
	Failures       int64   `json:"failures"`
	P95TTFTMs      float64 `json:"p95_ttft_ms"`
	InputTokens    int64   `json:"input_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	Cost           float64 `json:"cost"`
	RequestShare   float64 `json:"request_share"`
	CostShare      float64 `json:"cost_share"`
}

type routingSchemeAnalyticsResponse struct {
	SchemeID     int64                                 `json:"scheme_id"`
	Window       string                                `json:"window"`
	Since        time.Time                             `json:"since"`
	Requests     int64                                 `json:"requests"`
	Cost         float64                               `json:"cost"`
	InputTokens  int64                                 `json:"input_tokens"`
	OutputTokens int64                                 `json:"output_tokens"`
	Groups       []routingSchemeAnalyticsGroupResponse `json:"groups"`
}

// GetAnalytics handles GET /admin/routing-schemes/:id/analytics?window=24h.
func (h *RoutingSchemeHandler) GetAnalytics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid scheme ID")
		return
	}
	analytics, err := h.service.Analytics(c.Request.Context(), id, c.Query("window"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := routingSchemeAnalyticsResponse{
		SchemeID:     analytics.SchemeID,
		Window:       analytics.Window,
		Since:        analytics.Since,
		Requests:     analytics.Requests,
		Cost:         analytics.Cost,
		InputTokens:  analytics.InputTokens,
		OutputTokens: analytics.OutputTokens,
		Groups:       make([]routingSchemeAnalyticsGroupResponse, 0, len(analytics.Groups)),
	}
	for _, group := range analytics.Groups {
		out.Groups = append(out.Groups, routingSchemeAnalyticsGroupResponse{
			GroupID:        group.GroupID,
			GroupName:      group.GroupName,
			Platform:       group.Platform,
			RateMultiplier: group.RateMultiplier,
			Requests:       group.Requests,
			Successes:      group.Successes,
			Failures:       group.Failures,
			P95TTFTMs:      group.P95TTFTMs,
			InputTokens:    group.InputTokens,
			OutputTokens:   group.OutputTokens,
			Cost:           group.Cost,
			RequestShare:   group.RequestShare,
			CostShare:      group.CostShare,
		})
	}
	response.Success(c, out)
}

// List handles GET /admin/routing-schemes.
func (h *RoutingSchemeHandler) List(c *gin.Context) {
	includeDisabled := c.Query("include_disabled") == "true" || c.Query("include_disabled") == "1"
	schemes, err := h.service.List(c.Request.Context(), c.Query("platform"), includeDisabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]routingSchemeResponse, 0, len(schemes))
	for i := range schemes {
		out = append(out, h.toResponse(c, &schemes[i]))
	}
	response.Success(c, out)
}

// GetByID handles GET /admin/routing-schemes/:id.
func (h *RoutingSchemeHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid scheme ID")
		return
	}
	scheme, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toResponse(c, scheme))
}

// Create handles POST /admin/routing-schemes.
func (h *RoutingSchemeHandler) Create(c *gin.Context) {
	var req routingSchemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	scheme, err := h.service.Create(c.Request.Context(), req.toServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toResponse(c, scheme))
}

// Update handles PUT /admin/routing-schemes/:id.
func (h *RoutingSchemeHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid scheme ID")
		return
	}
	var req routingSchemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	scheme, err := h.service.Update(c.Request.Context(), id, req.toServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toResponse(c, scheme))
}

// Delete handles DELETE /admin/routing-schemes/:id.
func (h *RoutingSchemeHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid scheme ID")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Routing scheme deleted"})
}

func (r routingSchemeRequest) toServiceInput() service.RoutingSchemeInput {
	items := make([]service.RoutingSchemeItemInput, 0, len(r.Items))
	for _, item := range r.Items {
		items = append(items, service.RoutingSchemeItemInput{
			GroupID: item.GroupID,
			Weight:  item.Weight,
			Rules:   item.Rules,
		})
	}
	return service.RoutingSchemeInput{
		Name:        r.Name,
		Description: r.Description,
		Platform:    r.Platform,
		Status:      r.Status,
		SortOrder:   r.SortOrder,
		SortMode:    r.SortMode,
		Items:       items,
	}
}

func (h *RoutingSchemeHandler) toResponse(c *gin.Context, scheme *service.RoutingScheme) routingSchemeResponse {
	if scheme == nil {
		return routingSchemeResponse{}
	}
	scores := h.service.Scores(c.Request.Context(), scheme)
	items := make([]routingSchemeItemResponse, 0, len(scheme.Items))
	for _, item := range scheme.Items {
		score := scores[item.GroupID]
		rules := item.Rules
		if rules == nil {
			rules = []domain.RoutingSchemeRule{}
		}
		items = append(items, routingSchemeItemResponse{
			GroupID:        item.GroupID,
			GroupName:      item.GroupName,
			Platform:       item.Platform,
			RateMultiplier: item.RateMultiplier,
			Weight:         item.Weight,
			Position:       item.Position,
			Rules:          rules,
			Score:          score.Overall,
			P95TTFTMs:      score.P95TTFTMs,
			FailureRate:    score.FailureRate,
			FailureCount:   score.FailureCount,
			SampleCount:    score.SampleCount,
			ColdStart:      score.ColdStart,
		})
	}
	return routingSchemeResponse{
		ID:          scheme.ID,
		Name:        scheme.Name,
		Description: scheme.Description,
		Platform:    scheme.Platform,
		Status:      scheme.Status,
		SortOrder:   scheme.SortOrder,
		SortMode:    scheme.SortMode,
		Items:       items,
		CreatedAt:   scheme.CreatedAt,
		UpdatedAt:   scheme.UpdatedAt,
	}
}
