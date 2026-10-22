package service

import (
	"bytes"
	"context"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
)

func (s *GatewayService) sseStreamErrorEventFailover(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, rawData, model string, outputStarted bool) *UpstreamFailoverError {
	body := []byte(rawData)
	semanticStatus := http.StatusForbidden
	if !outputStarted && gjson.GetBytes(body, "error.type").String() == "overloaded_error" {
		semanticStatus = 529
		syntheticResp := &http.Response{
			StatusCode: semanticStatus,
			Header:     resp.Header.Clone(),
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		s.handleFailoverSideEffects(ctx, syntheticResp, account, model)
	}

	upstreamMsg := sanitizeUpstreamErrorMessage(
		strings.TrimSpace(extractUpstreamErrorMessage(body)),
	)

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(rawData, maxBytes)
	}

	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: semanticStatus,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "stream_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	logger.LegacyPrintf("service.gateway",
		"[Forward] SSE error event in stream: Account=%d(%s) RequestID=%s Body=%s",
		account.ID, account.Name, resp.Header.Get("x-request-id"),
		truncateString(rawData, 1000),
	)

	return &UpstreamFailoverError{
		StatusCode:   semanticStatus,
		ResponseBody: body,
	}
}
