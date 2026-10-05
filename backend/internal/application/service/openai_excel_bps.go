package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/basispoints"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// The caches are deliberately process-local. They retain only validated tool
// contracts and attachment IDs; the account ID and prompt cache key remain part
// of every scope so one account cannot replay another account's turn.
var excelBPSReplay basispoints.ReplayCache
var excelBPSCatalog basispoints.CatalogCache
var excelBPSAttachments basispoints.AttachmentCache

func excelBPSAccountID(account *Account, accessToken string) string {
	if account != nil {
		if id := strings.TrimSpace(account.GetChatGPTAccountID()); id != "" {
			return id
		}
	}
	claims, err := openai.DecodeIDToken(accessToken)
	if err != nil || claims == nil || claims.OpenAIAuth == nil {
		return ""
	}
	return strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
}

// Cache only turns with an explicit thread identity. A missing prompt cache key
// must never let two clients sharing one account inherit each other's tools.
func excelBPSCacheScope(accountID, apiKeyID int64, identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	return fmt.Sprintf("account:%d/key:%d/thread:%s", accountID, apiKeyID, identity)
}

func newExcelBPSRequest(ctx context.Context, body []byte, token, accountID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, basispoints.ResponsesURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Chatgpt-Account-Id", accountID)
	req.Header.Set("X-Openai-Account-Id", accountID)
	req.Header.Set("X-Basispoints-Auth-Mode", "chatgpt")
	req.Header.Set("X-Openai-Internal-Basispoints-Client-Product", "basispoints-excel-plugin")
	req.Header.Set("X-Openai-Internal-Basispoints-Client-Agent-Profile", "excel")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://bps.openai.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	return req, nil
}

// forwardExcelBPS is the account-scoped Excel/Basispoints Responses bridge.
// It intentionally uses the existing account HTTP transport so proxy, egress,
// concurrency, TLS and test doubles remain identical to other gateway paths.
func (s *OpenAIGatewayService) forwardExcelBPS(ctx context.Context, c *gin.Context, account *Account, body []byte, start time.Time) (*OpenAIForwardResult, error) {
	fail := func(status int, code, message string) (*OpenAIForwardResult, error) {
		if c != nil && !c.Writer.Written() {
			writeOpenAIResponsesFallbackError(c, status, "server_error", message)
		}
		return nil, fmt.Errorf("excel BPS %s", code)
	}
	if account == nil || !account.IsExcelBPSEnabled() {
		return fail(http.StatusForbidden, "disabled", "Excel BPS is not enabled for this account")
	}
	if reason := basispoints.NativeFallbackReason(body); reason != "" {
		return fail(http.StatusBadRequest, "unsupported_tool", "Excel BPS does not support this native tool: "+reason)
	}

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		return fail(http.StatusBadRequest, "invalid_request", "model is required")
	}
	mappedModel := strings.TrimSpace(account.GetMappedModel(originalModel))
	if mappedModel == "" {
		mappedModel = originalModel
	}
	body, err := sjson.SetBytes(body, "model", mappedModel)
	if err != nil {
		return fail(http.StatusBadRequest, "invalid_request", "invalid model request")
	}

	accessToken, _, err := s.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(accessToken) == "" {
		return fail(http.StatusBadGateway, "auth_unavailable", "account OAuth credential is unavailable")
	}
	accountID := excelBPSAccountID(account, accessToken)
	if accountID == "" {
		return fail(http.StatusBadRequest, "account_id_missing", "Excel BPS requires chatgpt_account_id")
	}

	identity := gjson.GetBytes(body, "prompt_cache_key").String()
	scope := excelBPSCacheScope(account.ID, getAPIKeyIDFromContext(c), identity)
	var replay *basispoints.ReplayCache
	var catalog *basispoints.CatalogCache
	if scope != "" {
		replay, catalog = &excelBPSReplay, &excelBPSCatalog
	}

	// NativeImages validates every inline image before any upload. This keeps
	// malformed requests atomic and allows the shared bridge to validate tools
	// against the same catalog after file IDs are inserted.
	images, err := basispoints.PrepareNativeImages(body)
	if err != nil {
		return fail(http.StatusBadRequest, "invalid_request", err.Error())
	}
	prepared, bridge, err := images.PrepareWithCatalog(scope, replay, catalog)
	if err != nil {
		var validationErr *basispoints.ContentValidationError
		if errors.As(err, &validationErr) {
			return fail(http.StatusBadRequest, "invalid_request", validationErr.Error())
		}
		return fail(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if images.HasImages() {
		attachmentScope := ""
		if scope != "" {
			attachmentScope = scope + "\x00" + accountID
		}
		bodyWithIDs, uploadErr := images.Upload(ctx, &excelBPSAttachments, attachmentScope, func(uploadCtx context.Context, image basispoints.InlineAttachment) (string, error) {
			return s.uploadExcelBPSAttachment(uploadCtx, account, accessToken, accountID, image)
		})
		if uploadErr != nil {
			return fail(http.StatusBadGateway, "attachment_error", "Excel BPS attachment upload failed")
		}
		prepared, bridge, err = bridge.Reprepare(bodyWithIDs)
		if err != nil {
			return fail(http.StatusBadRequest, "invalid_request", err.Error())
		}
	}

	requestCtx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI))
	req, err := newExcelBPSRequest(requestCtx, prepared, accessToken, accountID)
	if err != nil {
		return fail(http.StatusBadGateway, "request_build_failed", "Excel BPS request could not be prepared")
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	SetActualOpenAIUpstreamEndpoint(c, "/basispoints/api/responses")
	upstreamStart := time.Now()
	resp, err := s.doAccountHTTPUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return fail(http.StatusBadGateway, "transport_error", "Excel BPS connection failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512<<10))
		status := resp.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		return fail(status, "upstream_error", "Excel BPS rejected this request")
	}

	stream := gjson.GetBytes(body, "stream").Bool()
	result := &OpenAIForwardResult{
		Model: originalModel, BillingModel: originalModel, UpstreamModel: mappedModel,
		UpstreamEndpoint: "/basispoints/api/responses", Stream: stream,
		RequestID: resp.Header.Get("x-request-id"), Duration: time.Since(start),
	}
	// The bridge consumes SSE frames. A successful upstream JSON response is a
	// complete Responses object, not an SSE frame; read it directly before
	// constructing the bridge and wrap it if the client requested streaming.
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
		if readErr != nil {
			return result, readErr
		}
		if len(raw) > 16<<20 || !gjson.ValidBytes(raw) {
			return result, fmt.Errorf("excel BPS returned an invalid or oversized JSON response")
		}
		if status := gjson.GetBytes(raw, "status").String(); status != "" && status != "completed" {
			return result, fmt.Errorf("excel BPS returned a non-completed JSON response: %s", status)
		}
		result.ResponseID = extractOpenAIResponseIDFromJSONBytes(raw)
		result.UpstreamResponseModel = gjson.GetBytes(raw, "model").String()
		result.UpstreamTerminalEvent = "response.completed"
		if usage, ok := extractOpenAIUsageFromJSONBytes(raw); ok {
			result.Usage = usage
		}
		if stream {
			terminal, marshalErr := json.Marshal(gin.H{"type": "response.completed", "response": json.RawMessage(raw)})
			if marshalErr != nil {
				return result, marshalErr
			}
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			c.Writer.Header().Set("Cache-Control", "no-cache")
			c.Writer.Header().Set("X-Accel-Buffering", "no")
			_, _ = fmt.Fprintf(c.Writer, "event: response.completed\ndata: %s\n\n", terminal)
			c.Writer.Flush()
		} else {
			c.Data(http.StatusOK, "application/json", raw)
		}
		result.Duration = time.Since(start)
		s.bindHTTPResponseAccount(ctx, c, account, result.ResponseID)
		return result, nil
	}

	converted := bridge.StreamWithToolRepair(ctx, resp.Body, nil)
	defer func() { _ = converted.Close() }()
	scanner := s.newUpstreamSSEScanner(converted)
	var completed []byte
	terminal := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data: ")))
			if len(payload) > 0 && string(payload) != "[DONE]" {
				s.parseSSEUsageBytes(payload, &result.Usage)
				eventType := gjson.GetBytes(payload, "type").String()
				if result.FirstTokenMs == nil && (eventType == "response.output_text.delta" || eventType == "response.output_item.added") {
					ms := int(time.Since(start).Milliseconds())
					result.FirstTokenMs = &ms
				}
				switch eventType {
				case "response.completed", "response.failed", "response.incomplete", "error":
					terminal = eventType
					completed = []byte(gjson.GetBytes(payload, "response").Raw)
					result.ResponseID = extractOpenAIResponseIDFromJSONBytes(payload)
					result.UpstreamResponseModel = gjson.GetBytes(payload, "response.model").String()
				}
			}
		}
		if stream {
			if !c.Writer.Written() {
				c.Writer.Header().Set("Content-Type", "text/event-stream")
				c.Writer.Header().Set("Cache-Control", "no-cache")
				c.Writer.Header().Set("Connection", "keep-alive")
				c.Writer.Header().Set("X-Accel-Buffering", "no")
				c.Writer.WriteHeader(http.StatusOK)
				MarkResponseCommitted(c)
			}
			if _, err := c.Writer.WriteString(line + "\n"); err != nil {
				result.ClientDisconnect = true
				return result, err
			}
			if line == "" {
				c.Writer.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("excel BPS stream read: %w", err)
	}
	result.Duration = time.Since(start)
	result.UpstreamTerminalEvent = terminal
	if terminal != "response.completed" {
		if stream {
			return result, fmt.Errorf("excel BPS terminal event: %s", terminal)
		}
		return result, fmt.Errorf("excel BPS response did not complete")
	}
	if !stream {
		if len(completed) == 0 || !gjson.ValidBytes(completed) {
			return result, fmt.Errorf("excel BPS completed event did not contain a response")
		}
		c.Data(http.StatusOK, "application/json", completed)
	}
	s.bindHTTPResponseAccount(ctx, c, account, result.ResponseID)
	return result, nil
}

func (s *OpenAIGatewayService) uploadExcelBPSAttachment(ctx context.Context, account *Account, token, accountID string, image basispoints.InlineAttachment) (string, error) {
	reader, contentType, length, err := image.Multipart()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI)), http.MethodPost, basispoints.AttachmentsURL, reader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Chatgpt-Account-Id", accountID)
	req.Header.Set("X-Openai-Account-Id", accountID)
	req.Header.Set("X-Basispoints-Auth-Mode", "chatgpt")
	req.Header.Set("X-Openai-Internal-Basispoints-Client-Product", "basispoints-excel-plugin")
	req.Header.Set("X-Openai-Internal-Basispoints-Client-Agent-Profile", "excel")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Encoding", "identity")
	req.ContentLength = length
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doAccountHTTPUpstream(req, proxyURL, account)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("attachment upload returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		OpenAIFileID string `json:"openai_file_id"`
		ID           string `json:"id"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	id := payload.OpenAIFileID
	if id == "" {
		id = payload.ID
	}
	if !basispoints.ValidAttachmentID(id) {
		return "", fmt.Errorf("invalid attachment id")
	}
	return id, nil
}
