package admin

import (
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type EgressHandler struct {
	service   *moduleegress.Service
	heControl *moduleegress.HETunnelControlService
	cfg       *config.Config
}

type updateEgressRuntimeInput struct {
	Enabled bool `json:"enabled"`
}

func NewEgressHandler(service *moduleegress.Service, heControl *moduleegress.HETunnelControlService, cfg *config.Config) *EgressHandler {
	return &EgressHandler{service: service, heControl: heControl, cfg: cfg}
}

func (h *EgressHandler) Runtime(c *gin.Context) {
	enabled := false
	freeBind := true
	secretConfigured := false
	if h.cfg != nil {
		freeBind = h.cfg.IPv6Egress.FreeBind
	}
	if h.service != nil {
		enabled = h.service.IsEnabled()
		secretConfigured = h.service.SecretConfigured()
	}
	ready := h.service != nil && h.service.RuntimeReady() == nil
	detectedPrefix := ""
	if runtime.GOOS == "linux" {
		if detected, err := platformegress.DetectIPv6Network(); err == nil && detected != nil {
			detectedPrefix = detected.Prefix.String()
		}
	}
	// An empty legacy probe URL uses the built-in api64.ipify.org default.
	probeConfigured := true
	if h.cfg != nil && strings.TrimSpace(h.cfg.IPv6Egress.ProbeURL) != "" {
		probeURL, err := url.Parse(strings.TrimSpace(h.cfg.IPv6Egress.ProbeURL))
		probeConfigured = err == nil && strings.EqualFold(probeURL.Scheme, "https") && strings.TrimSpace(probeURL.Hostname()) != ""
	}
	response.Success(c, gin.H{
		"enabled":           enabled,
		"supported":         runtime.GOOS == "linux",
		"platform":          runtime.GOOS,
		"freebind":          freeBind,
		"secret_configured": secretConfigured,
		"fail_closed":       true,
		"ready":             ready,
		"reconcile_interval_seconds": func() int {
			if h.cfg == nil {
				return 0
			}
			return h.cfg.IPv6Egress.ReconcileIntervalSeconds
		}(),
		"probe_configured": probeConfigured,
		"control_enabled":  h.cfg != nil && h.cfg.IPv6Egress.ControlEnabled,
		"detected_prefix":  detectedPrefix,
	})
}

func (h *EgressHandler) UpdateRuntime(c *gin.Context) {
	var input updateEgressRuntimeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "IPv6 egress service is unavailable")
		return
	}
	if err := h.service.SetEnabled(c.Request.Context(), input.Enabled); err != nil {
		h.writeError(c, err)
		return
	}
	h.Runtime(c)
}

func (h *EgressHandler) AutoConfigure(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "IPv6 egress service is unavailable")
		return
	}
	result, err := h.service.AutoConfigure(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *EgressHandler) GetHETunnel(c *gin.Context) {
	snapshot, err := h.heControl.Get(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, snapshot)
}

func (h *EgressHandler) SaveHETunnel(c *gin.Context) {
	var input moduleegress.SaveHETunnelConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	snapshot, err := h.heControl.Save(c.Request.Context(), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, snapshot)
}

func (h *EgressHandler) HETunnelAction(c *gin.Context) {
	snapshot, err := h.heControl.Request(c.Request.Context(), c.Param("action"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Accepted(c, snapshot)
}

func (h *EgressHandler) ListPools(c *gin.Context) {
	pools, err := h.service.ListPools(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, pools)
}

func (h *EgressHandler) CreatePool(c *gin.Context) {
	var input moduleegress.CreatePoolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	pool, err := h.service.CreatePool(c.Request.Context(), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Created(c, pool)
}

func (h *EgressHandler) UpdatePool(c *gin.Context) {
	id, ok := parseEgressID(c, "id")
	if !ok {
		return
	}
	var input moduleegress.UpdatePoolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	pool, err := h.service.UpdatePool(c.Request.Context(), id, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, pool)
}

func (h *EgressHandler) DeletePool(c *gin.Context) {
	id, ok := parseEgressID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeletePool(c.Request.Context(), id); err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *EgressHandler) ListBindings(c *gin.Context) {
	page, pageSize := response.ParsePaginationWithMax(c, 200)
	result, err := h.service.ListBindings(c.Request.Context(), (page-1)*pageSize, pageSize, c.Query("search"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Paginated(c, result.Items, result.Total, page, pageSize)
}

func (h *EgressHandler) SetAccountRoute(c *gin.Context) {
	accountID, ok := parseEgressID(c, "id")
	if !ok {
		return
	}
	var input moduleegress.SetAccountRouteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	binding, err := h.service.SetAccountRoute(c.Request.Context(), accountID, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"mode": input.Mode, "binding": binding})
}

func (h *EgressHandler) RotateBinding(c *gin.Context) {
	accountID, ok := parseEgressID(c, "id")
	if !ok {
		return
	}
	binding, err := h.service.RotateBinding(c.Request.Context(), accountID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, binding)
}

func (h *EgressHandler) ProbeAccount(c *gin.Context) {
	accountID, ok := parseEgressID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.ProbeAccount(c.Request.Context(), accountID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *EgressHandler) ReconcileDefault(c *gin.Context) {
	limit := 1000
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	completed, err := h.service.ReconcileDefault(c.Request.Context(), limit)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"allocated": completed, "limit": limit})
}

func (h *EgressHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, moduleegress.ErrPoolNotFound), errors.Is(err, moduleegress.ErrBindingNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, moduleegress.ErrHETunnelControlUnavailable):
		response.Error(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, moduleegress.ErrPoolInUse), errors.Is(err, moduleegress.ErrPoolOverlap), errors.Is(err, moduleegress.ErrBindingChanged), errors.Is(err, moduleegress.ErrAddressConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, moduleegress.ErrPoolDisabled), errors.Is(err, moduleegress.ErrPoolUnhealthy), errors.Is(err, moduleegress.ErrAllocationDisabled), errors.Is(err, moduleegress.ErrRuntimeUnavailable),
		errors.Is(err, moduleegress.ErrAutoConfigure), errors.Is(err, platformegress.ErrIPv6AutoDetect),
		errors.Is(err, platformegress.ErrIPv6Disabled), errors.Is(err, platformegress.ErrIPv6Unsupported),
		errors.Is(err, platformegress.ErrIPv6Destination), errors.Is(err, platformegress.ErrIPv6SourceRequired):
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
	default:
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "too small") {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
	}
}

func parseEgressID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return 0, false
	}
	return id, true
}
