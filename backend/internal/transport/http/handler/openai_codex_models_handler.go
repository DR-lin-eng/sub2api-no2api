package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// GET /backend-api/codex/models lands here. The ordinary /models and
// /v1/models routes are local model-list endpoints and never use this handler.
// The manifest is proxied verbatim from the selected account's ChatGPT backend
// or custom API key upstream. API key manifests use a short-lived,
// asynchronously revalidated cache to tolerate canceled client requests.
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
			accountRelease, err = acquireMetadataAccountSlot(c, h.concurrencyHelper, h.cfg, account)
			if err != nil {
				h.handleConcurrencyError(c, err, "account", false)
				return
			}
		}

		manifest, err := func() (*service.CodexModelsManifest, error) {
			if accountRelease != nil {
				defer accountRelease()
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

		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
		}
		if manifest.NotModified {
			c.Status(http.StatusNotModified)
			return
		}
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
	}
}
