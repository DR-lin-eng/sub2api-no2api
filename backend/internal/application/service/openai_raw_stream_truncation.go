package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIRawStreamTruncatedUpstreamMessage = "Upstream Chat Completions stream ended before any terminal chunk"

type openAIRawStreamTerminalState struct {
	sawDataLine     bool
	sawDone         bool
	sawUsage        bool
	sawFinishReason bool
}

func (t *openAIRawStreamTerminalState) ObserveDataLine(payload string) {
	if t == nil {
		return
	}
	t.sawDataLine = true
	if payload == "[DONE]" {
		t.sawDone = true
		return
	}
	if usage := gjson.Get(payload, "usage"); usage.Exists() && usage.IsObject() {
		t.sawUsage = true
	}
	gjson.Get(payload, "choices").ForEach(func(_, choice gjson.Result) bool {
		if strings.TrimSpace(choice.Get("finish_reason").String()) != "" {
			t.sawFinishReason = true
			return false
		}
		return true
	})
}

func (t *openAIRawStreamTerminalState) Terminated() bool {
	return t != nil && (t.sawDone || t.sawUsage || t.sawFinishReason)
}

func (t *openAIRawStreamTerminalState) IsTruncated(clientOutputStarted bool) bool {
	if t == nil || t.Terminated() {
		return false
	}
	return t.sawDataLine || !clientOutputStarted
}

func newOpenAIRawStreamTruncatedFailoverError(
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
	cause error,
) *UpstreamFailoverError {
	recordOpenAIRawStreamTruncation(c, account, upstreamRequestID, cause, "failover")
	headers := http.Header{}
	if id := strings.TrimSpace(upstreamRequestID); id != "" {
		headers.Set("x-request-id", id)
	}
	return &UpstreamFailoverError{
		StatusCode:      http.StatusBadGateway,
		ResponseBody:    openAIRawStreamTruncatedErrorBody(cause),
		ResponseHeaders: headers,
	}
}

func recordOpenAIRawStreamTruncation(
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
	cause error,
	kind string,
) {
	if c == nil {
		return
	}
	message := openAIRawStreamTruncatedMessage(cause)
	platform := PlatformOpenAI
	accountID := int64(0)
	accountName := ""
	if account != nil {
		platform = account.Platform
		accountID = account.ID
		accountName = account.Name
	}
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           platform,
		AccountID:          accountID,
		AccountName:        accountName,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
		Kind:               kind,
		Message:            message,
	})
}

func openAIRawStreamTruncatedMessage(cause error) string {
	if cause == nil || errors.Is(cause, ErrOpenAIUpstreamStreamTruncated) {
		return openAIRawStreamTruncatedUpstreamMessage
	}
	return openAIRawStreamTruncatedUpstreamMessage + ": " + cause.Error()
}

func openAIRawStreamTruncatedErrorBody(cause error) []byte {
	code, message := classifyOpenAIUpstreamStreamReadError(cause)
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"code":    code,
			"message": message,
		},
	})
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","code":"upstream_stream_truncated","message":"Upstream response stream ended before completion"}}`)
	}
	return body
}
