package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func validatePresentationSettingsUpdate(c *gin.Context, prepared *preparedSettingsUpdate) bool {
	req := prepared.request
	previousSettings := prepared.previousSettings

	// “购买订阅”页面配置验证
	purchaseEnabled := previousSettings.PurchaseSubscriptionEnabled
	if req.PurchaseSubscriptionEnabled != nil {
		purchaseEnabled = *req.PurchaseSubscriptionEnabled
	}
	purchaseURL := previousSettings.PurchaseSubscriptionURL
	if req.PurchaseSubscriptionURL != nil {
		purchaseURL = strings.TrimSpace(*req.PurchaseSubscriptionURL)
	}

	// - 启用时要求 URL 合法且非空
	// - 禁用时允许为空；若提供了 URL 也做基本校验，避免误配置
	if purchaseEnabled {
		if purchaseURL == "" {
			response.BadRequest(c, "Purchase Subscription URL is required when enabled")
			return false
		}
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return false
		}
	} else if purchaseURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return false
		}
	}

	// Frontend URL 验证
	req.FrontendURL = strings.TrimSpace(req.FrontendURL)
	if req.FrontendURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(req.FrontendURL); err != nil {
			response.BadRequest(c, "Frontend URL must be an absolute http(s) URL")
			return false
		}
	}

	// 自定义菜单项验证
	const (
		maxCustomMenuItems    = 20
		maxMenuItemLabelLen   = 50
		maxMenuItemURLLen     = 2048
		maxMenuItemIconSVGLen = 10 * 1024 // 10KB
		maxMenuItemIDLen      = 32
	)

	customMenuJSON := previousSettings.CustomMenuItems
	if req.CustomMenuItems != nil {
		items := *req.CustomMenuItems
		if len(items) > maxCustomMenuItems {
			response.BadRequest(c, "Too many custom menu items (max 20)")
			return false
		}
		for i, item := range items {
			if strings.TrimSpace(item.Label) == "" {
				response.BadRequest(c, "Custom menu item label is required")
				return false
			}
			if len(item.Label) > maxMenuItemLabelLen {
				response.BadRequest(c, "Custom menu item label is too long (max 50 characters)")
				return false
			}
			urlTrimmed := strings.TrimSpace(item.URL)
			if strings.HasPrefix(urlTrimmed, "md:") {
				// Markdown page mode: URL = "md:<slug>"
				slug := strings.TrimPrefix(urlTrimmed, "md:")
				if slug == "" {
					response.BadRequest(c, "Custom menu item markdown slug cannot be empty (use md:slug format)")
					return false
				}
			} else {
				if urlTrimmed == "" {
					response.BadRequest(c, "Custom menu item URL is required (use md:slug for markdown pages)")
					return false
				}
				if len(item.URL) > maxMenuItemURLLen {
					response.BadRequest(c, "Custom menu item URL is too long (max 2048 characters)")
					return false
				}
				if err := config.ValidateAbsoluteHTTPURL(urlTrimmed); err != nil {
					response.BadRequest(c, "Custom menu item URL must be an absolute http(s) URL or md:<slug>")
					return false
				}
			}
			if item.Visibility != "user" && item.Visibility != "admin" {
				response.BadRequest(c, "Custom menu item visibility must be 'user' or 'admin'")
				return false
			}
			if len(item.IconSVG) > maxMenuItemIconSVGLen {
				response.BadRequest(c, "Custom menu item icon SVG is too large (max 10KB)")
				return false
			}
			// Auto-generate ID if missing
			if strings.TrimSpace(item.ID) == "" {
				id, err := generateMenuItemID()
				if err != nil {
					response.Error(c, http.StatusInternalServerError, "Failed to generate menu item ID")
					return false
				}
				items[i].ID = id
			} else if len(item.ID) > maxMenuItemIDLen {
				response.BadRequest(c, "Custom menu item ID is too long (max 32 characters)")
				return false
			} else if !menuItemIDPattern.MatchString(item.ID) {
				response.BadRequest(c, "Custom menu item ID contains invalid characters (only a-z, A-Z, 0-9, - and _ are allowed)")
				return false
			}
		}
		// ID uniqueness check
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				response.BadRequest(c, "Duplicate custom menu item ID: "+item.ID)
				return false
			}
			seen[item.ID] = struct{}{}
		}
		menuBytes, err := json.Marshal(items)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom menu items")
			return false
		}
		customMenuJSON = string(menuBytes)
	}

	// 自定义端点验证
	const (
		maxCustomEndpoints        = 10
		maxEndpointNameLen        = 50
		maxEndpointURLLen         = 2048
		maxEndpointDescriptionLen = 200
	)

	customEndpointsJSON := previousSettings.CustomEndpoints
	if req.CustomEndpoints != nil {
		endpoints := *req.CustomEndpoints
		if len(endpoints) > maxCustomEndpoints {
			response.BadRequest(c, "Too many custom endpoints (max 10)")
			return false
		}
		for _, ep := range endpoints {
			if strings.TrimSpace(ep.Name) == "" {
				response.BadRequest(c, "Custom endpoint name is required")
				return false
			}
			if len(ep.Name) > maxEndpointNameLen {
				response.BadRequest(c, "Custom endpoint name is too long (max 50 characters)")
				return false
			}
			if strings.TrimSpace(ep.Endpoint) == "" {
				response.BadRequest(c, "Custom endpoint URL is required")
				return false
			}
			if len(ep.Endpoint) > maxEndpointURLLen {
				response.BadRequest(c, "Custom endpoint URL is too long (max 2048 characters)")
				return false
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(ep.Endpoint)); err != nil {
				response.BadRequest(c, "Custom endpoint URL must be an absolute http(s) URL")
				return false
			}
			if len(ep.Description) > maxEndpointDescriptionLen {
				response.BadRequest(c, "Custom endpoint description is too long (max 200 characters)")
				return false
			}
		}
		endpointBytes, err := json.Marshal(endpoints)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom endpoints")
			return false
		}
		customEndpointsJSON = string(endpointBytes)
	}

	prepared.purchaseEnabled = purchaseEnabled
	prepared.purchaseURL = purchaseURL
	prepared.customMenuJSON = customMenuJSON
	prepared.customEndpointsJSON = customEndpointsJSON
	return true
}
