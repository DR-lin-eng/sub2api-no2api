package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIImagesReadErrorBody struct {
	payload []byte
	err     error
	sent    bool
}

func (b *openAIImagesReadErrorBody) Read(p []byte) (int, error) {
	if !b.sent && len(b.payload) > 0 {
		b.sent = true
		return copy(p, b.payload), nil
	}
	return 0, b.err
}

func (b *openAIImagesReadErrorBody) Close() error { return nil }

func TestOpenAIImagesOAuthBodyReadTransportErrorFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Request-Id": []string{"req_h2_read_failure"},
			"X-Upstream":   []string{"preserved"},
		},
		Body: &openAIImagesReadErrorBody{err: errors.New("stream error: stream ID 11; INTERNAL_ERROR; received from peer")},
	}
	account := &Account{ID: 5400, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{}
	before := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)

	_, _, _, readErr := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2")
	require.Error(t, readErr)
	err := svc.handleOpenAIImagesOAuthResponseError(
		context.Background(), c, account, "gpt-image-2", "https://api.openai.com/v1/responses", resp, before, readErr,
	)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.JSONEq(t, `{"error":{"type":"upstream_error","code":"upstream_http2_stream_error","message":"Upstream HTTP/2 stream failed"}}`, string(failoverErr.ResponseBody))
	require.Equal(t, "req_h2_read_failure", failoverErr.ResponseHeaders.Get("x-request-id"))
	require.Equal(t, "preserved", failoverErr.ResponseHeaders.Get("x-upstream"))
	resp.Header.Set("X-Upstream", "mutated")
	require.Equal(t, "preserved", failoverErr.ResponseHeaders.Get("x-upstream"))
}

func TestOpenAIImagesOAuthStreamingReadErrorBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"req_stream_read_failure"},
		},
		Body: &openAIImagesReadErrorBody{
			payload: []byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n"),
			err:     io.ErrUnexpectedEOF,
		},
	}
	account := &Account{ID: 5401, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{}
	before := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)

	_, _, _, _, readErr := svc.handleOpenAIImagesOAuthStreamingResponse(resp, c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
	require.Error(t, readErr)
	require.Empty(t, recorder.Body.String(), "transport errors before image output must remain replayable")
	err := svc.handleOpenAIImagesOAuthResponseError(
		context.Background(), c, account, "gpt-image-2", "https://api.openai.com/v1/responses", resp, before, readErr,
	)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, OpenAIUpstreamStreamReadErrorCode, openAIImagesTestErrorCode(failoverErr.ResponseBody))
}

func TestOpenAIImagesOAuthBodyReadErrorsNotMisclassified(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "context canceled", err: context.Canceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded},
		{name: "response too large", err: fmt.Errorf("%w: limit=1", ErrUpstreamResponseBodyTooLarge)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, shouldClassifyOpenAIUpstreamStreamReadError(tt.err))
		})
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, shouldClassifyOpenAIUpstreamStreamReadError(io.ErrUnexpectedEOF, canceledCtx))
	require.True(t, shouldClassifyOpenAIUpstreamStreamReadError(io.ErrUnexpectedEOF))
}

func TestOpenAIImagesOAuthTransportErrorAfterDownstreamWriteDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	before := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	_, writeErr := c.Writer.Write([]byte("downstream image bytes"))
	require.NoError(t, writeErr)
	classifiedErr := newOpenAIUpstreamStreamReadError(io.ErrUnexpectedEOF)
	account := &Account{ID: 5402, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	resp := &http.Response{Header: http.Header{"X-Request-Id": []string{"req_after_write"}}}

	err := (&OpenAIGatewayService{}).handleOpenAIImagesOAuthResponseError(
		context.Background(), c, account, "gpt-image-2", "", resp, before, classifiedErr,
	)

	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.ErrorIs(t, err, classifiedErr)
}

func openAIImagesTestErrorCode(body []byte) string {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Error.Code
}
