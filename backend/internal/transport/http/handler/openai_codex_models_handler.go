package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. ChatGPT manifests are proxied verbatim; custom API key manifests receive
// provider-compatibility normalization and use a short-lived, asynchronously
// revalidated cache to tolerate canceled client requests.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI groups")
		return
	}

	priorityAdmissionEnabled := metadataPriorityAdmissionEnabled(c, h.concurrencyHelper)
	if priorityAdmissionEnabled {
		authSubject, ok := middleware2.GetAuthSubjectFromContext(c)
		if !ok {
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
			return
		}
		userRelease, err := acquireMetadataUserSlot(c, h.concurrencyHelper, authSubject)
		if err != nil {
			h.handleConcurrencyError(c, err, "user", false)
			return
		}
		if userRelease != nil {
			defer userRelease()
		}
	}

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	var failedAccountIDs map[int64]struct{}
	switchCount := 0
	var lastUpstreamErr error

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
		}
		// 让 ops 错误日志携带实际选中的上游账号，便于定位失效账号（#4544）。
		setOpsSelectedAccount(c, account.ID, account.Platform)

		var accountRelease func()
		if priorityAdmissionEnabled {
			accountRelease, err = acquireMetadataAccountSlot(c, h.concurrencyHelper, h.cfg, account, h.gatewayService, apiKey.GroupID)
			if err != nil {
				if errors.Is(err, service.ErrAccountSchedulingChanged) {
					addFailedAccountID(&failedAccountIDs, account.ID)
					continue
				}
				h.handleConcurrencyError(c, err, "account", false)
				return
			}
		}

		manifest, err := func() (*service.CodexModelsManifest, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			if apiKey.Group.ModelAllowlistEnabled() {
				return h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), "")
			}
			return h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), c.GetHeader("If-None-Match"))
		}()
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				addFailedAccountID(&failedAccountIDs, account.ID)
				switchCount++
				lastUpstreamErr = err
				continue
			}
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}
		if manifest.NotModified {
			if manifest.ETag != "" {
				c.Header("ETag", manifest.ETag)
			}
			c.Status(http.StatusNotModified)
			return
		}
		if apiKey.Group.ModelAllowlistEnabled() {
			filtered, filterErr := filterCodexManifestByAllowlist(manifest.Body, apiKey.Group.ModelAllowlist)
			if filterErr != nil {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "invalid Codex models manifest")
				return
			}
			manifest.Body = filtered
			manifest.ETag = ""
		}

		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
		}
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
	}
}

func filterCodexManifestByAllowlist(body []byte, allowlist service.GroupModelAllowlist) ([]byte, error) {
	if !allowlist.Enabled {
		return body, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var models []json.RawMessage
	if raw, ok := envelope["models"]; ok {
		if err := json.Unmarshal(raw, &models); err != nil {
			return nil, err
		}
	}
	filtered := models[:0]
	for _, raw := range models {
		var item struct {
			Slug string `json:"slug"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		model := strings.TrimSpace(item.Slug)
		if model == "" {
			model = strings.TrimSpace(item.ID)
		}
		if model != "" && allowlist.Allows(model) {
			filtered = append(filtered, raw)
		}
	}
	envelope["models"], _ = json.Marshal(filtered)
	return json.Marshal(envelope)
}
