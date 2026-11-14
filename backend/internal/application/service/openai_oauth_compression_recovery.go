package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIOAuthCompressionRecoveryKey = "openai_oauth_compression_recovery_tried"
const openAIOAuthIdentityEncodingAccountKey = "openai_oauth_identity_encoding_account"

// Keep compression enabled for every new request. Some relays reject a valid
// zstd frame with only {"detail":"Bad Request"}; on a replay-safe request this
// permits one encoding-only recovery, without changing account settings or
// guessing which input field to remove. The Gin flag survives account switches
// and the passthrough transport retry loop, so it cannot replenish the budget.
func (s *OpenAIGatewayService) doOpenAIResponsesWithCompressionRecovery(
	ctx context.Context,
	c *gin.Context,
	req *http.Request,
	proxyURL string,
	account *Account,
	stream bool,
	canonicalBody []byte,
	budget *openAIPassthroughRetryBudget,
) (*http.Response, error) {
	// An authentication or stream recovery may rebuild this logical request
	// after zstd was already rejected. Keep its proven encoding for that account;
	// a new request (or a different account) still starts with zstd.
	if c != nil && account != nil && account.IsOpenAIOAuth() && req != nil && req.GetBody != nil && strings.EqualFold(req.Header.Get("Content-Encoding"), "zstd") {
		if identityAccount, _ := c.Get(openAIOAuthIdentityEncodingAccountKey); identityAccount == account.ID {
			identityReq, _, decodeErr := newOpenAIIdentityEncodingRequest(req)
			if decodeErr != nil {
				return nil, decodeErr
			}
			req = identityReq
		}
	}
	resp, err := s.doOpenAIResponsesUpstream(ctx, c, req, proxyURL, account, stream)
	if err != nil || resp == nil || resp.Body == nil ||
		(resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnsupportedMediaType) ||
		!s.openAIOAuthCompressionRecoveryAllowed(ctx, c, req, account) {
		return resp, err
	}

	limit := openAIUpstreamErrorBodyReadLimitForConfig(s.cfg)
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	if readErr != nil || int64(len(responseBody)) > limit || !isOpenAIOAuthCompressionRejection(resp.StatusCode, responseBody) ||
		!openAIWSHTTPReplaySafeBody(canonicalBody) {
		return resp, nil
	}

	// Decode the exact wire snapshot. Rebuilding through Forward could change
	// metadata, continuation IDs or headers; the encoding is the only difference
	// here. Reuse the bounded ingress decoder and keep GetBody/length consistent.
	retryReq, body, decodeErr := newOpenAIIdentityEncodingRequest(req)
	if decodeErr != nil {
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI] Rejected zstd request snapshot is invalid: account_id=%d status=%d error=%v",
			account.ID, resp.StatusCode, decodeErr)
		return resp, nil
	}
	if !openAIWSHTTPReplaySafeBody(body) ||
		!s.openAIOAuthCompressionRecoveryAllowed(ctx, c, req, account) {
		return resp, nil
	}
	if budget != nil {
		if _, _, reserved := budget.reserve(); !reserved {
			// Preserve the original rejection when there is no send budget left.
			return resp, nil
		}
	}
	c.Set(openAIOAuthCompressionRecoveryKey, true)
	c.Set(openAIOAuthIdentityEncodingAccountKey, account.ID)
	_ = resp.Body.Close()
	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI] Retrying rejected zstd request once with identity encoding: account_id=%d status=%d upstream_request_id=%s pre_compression_bytes=%d post_compression_bytes=%d json_sha256=%x",
		account.ID, resp.StatusCode, resp.Header.Get("x-request-id"), len(body), req.ContentLength, sha256.Sum256(body))
	// This remains on the same selected account/proxy and passes through the
	// ordinary deadline, keepalive, OAuth admission and per-send proof lifecycle.
	return s.doOpenAIResponsesUpstream(ctx, c, retryReq, proxyURL, account, stream)
}

func newOpenAIIdentityEncodingRequest(req *http.Request) (*http.Request, []byte, error) {
	retryReq := req.Clone(req.Context())
	var err error
	retryReq.Body, err = req.GetBody()
	if err != nil {
		return nil, nil, err
	}
	// Validate the bytes actually sent, rather than trusting the declared size
	// or accepting a successful JSON prefix from a truncated/stacked encoding.
	snapshot := &openAICompressionSnapshotReader{ReadCloser: retryReq.Body}
	retryReq.Body = snapshot
	body, err := httputil.ReadRequestBodyWithPrealloc(retryReq)
	_ = retryReq.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	if req.ContentLength >= 0 && req.ContentLength != snapshot.bytesRead {
		return nil, nil, errors.New("compressed request snapshot length mismatch")
	}
	if length := req.Header.Get("Content-Length"); length != "" {
		declared, err := strconv.ParseInt(length, 10, 64)
		if err != nil || declared != snapshot.bytesRead {
			return nil, nil, errors.New("compressed request snapshot Content-Length mismatch")
		}
	}
	if !utf8.Valid(body) || !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, nil, errors.New("compressed request snapshot is not a UTF-8 JSON object")
	}
	retryReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	retryReq.Body, _ = retryReq.GetBody()
	retryReq.TransferEncoding = nil
	return retryReq, body, nil
}

type openAICompressionSnapshotReader struct {
	io.ReadCloser
	bytesRead int64
}

func (r *openAICompressionSnapshotReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (s *OpenAIGatewayService) openAIOAuthCompressionRecoveryAllowed(ctx context.Context, c *gin.Context, req *http.Request, account *Account) bool {
	if ctx == nil || ctx.Err() != nil || c == nil || c.Request == nil || c.Request.Context().Err() != nil ||
		account == nil || !account.IsOpenAIOAuth() || !isCodexRequestCompressionEnabled(account) ||
		req == nil || req.GetBody == nil || req.Context().Err() != nil ||
		!strings.EqualFold(strings.TrimSpace(req.Header.Get("Content-Encoding")), "zstd") ||
		s.IsDistillationGroupRequest(c, account) || codexContinuationRecoveryForbidden(c) || codexContinuationRequiresExactConnection(c) ||
		IsResponseCommitted(c) || OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0 {
		return false
	}
	tried, _ := c.Get(openAIOAuthCompressionRecoveryKey)
	return tried != true
}

// The unstructured error reported in #8636 contains no field diagnosis. Keep
// the match exact: structured errors, context limits, policy failures and
// unknown JSON fields must retain their existing handling. An explicit 415
// rejection of zstd/Content-Encoding is also an encoding negotiation failure.
func isOpenAIOAuthCompressionRejection(status int, body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	object := gjson.ParseBytes(body)
	if !object.IsObject() {
		return false
	}
	detail, fields := "", 0
	valid := true
	object.ForEach(func(key, value gjson.Result) bool {
		fields++
		if key.String() != "detail" || value.Type != gjson.String {
			valid = false
		}
		detail = strings.TrimSpace(value.String())
		return true
	})
	if !valid || fields != 1 {
		return false
	}
	if status == http.StatusBadRequest {
		return strings.EqualFold(detail, "Bad Request")
	}
	if status != http.StatusUnsupportedMediaType {
		return false
	}
	detail = strings.ToLower(detail)
	return strings.Contains(detail, "unsupported") &&
		(strings.Contains(detail, "zstd") || strings.Contains(detail, "content-encoding"))
}
