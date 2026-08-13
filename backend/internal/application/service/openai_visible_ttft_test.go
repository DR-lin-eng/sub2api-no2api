package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVisibleOutputClassification(t *testing.T) {
	for _, tt := range []struct {
		name      string
		data      string
		eventType string
		want      bool
	}{
		{name: "keepalive", data: `{"type":"keepalive"}`, want: false},
		{name: "created", data: `{"type":"response.created"}`, want: false},
		{name: "empty output item", data: `{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`, want: false},
		{name: "empty delta", data: `{"type":"response.output_text.delta","delta":""}`, want: false},
		{name: "text delta", data: `{"type":"response.output_text.delta","delta":"test output"}`, want: true},
		{name: "tool arguments", data: `{"type":"response.function_call_arguments.delta","delta":"{}"}`, want: true},
		{name: "partial image", data: `{"type":"response.image_generation_call.partial_image","partial_image_b64":"dGVzdA=="}`, want: true},
		{name: "completed image item", data: `{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"dGVzdA=="}}`, want: true},
		{name: "completed usage only", data: `{"type":"response.completed","response":{"usage":{"input_tokens":1}}}`, want: false},
		{name: "completed with text", data: `{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"test output"}]}]}}`, want: true},
		{name: "done marker", data: `[DONE]`, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIStreamDataStartsVisibleOutput(tt.data, tt.eventType))
		})
	}
}

func TestOpenAITTFTMeasurementModeClassifiesLegacySemanticOutput(t *testing.T) {
	data := `{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"encrypted"}}`
	require.False(t, openAIStreamDataStartsTTFT(data, "response.output_item.added", true))
	require.True(t, openAIStreamDataStartsTTFT(data, "response.output_item.added", false))
}

func TestOpenAIResponsesTTFTMeasurementMode(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			legacy := runSyntheticTTFTStream(t, passthrough, false, 120*time.Millisecond, 0,
				`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"encrypted"}}`,
				`{"type":"response.output_text.delta","delta":"test output"}`)
			current := runSyntheticTTFTStream(t, passthrough, true, 120*time.Millisecond, 0,
				`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"encrypted"}}`,
				`{"type":"response.output_text.delta","delta":"test output"}`)

			require.NotNil(t, legacy.firstTokenMs)
			require.NotNil(t, current.firstTokenMs)
			require.Less(t, *legacy.firstTokenMs, 100)
			require.GreaterOrEqual(t, *current.firstTokenMs, 100)
		})
	}
}

func TestOpenAIResponsesTTFTStartsAtVisibleOutput(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			result := runSyntheticVisibleTTFTStream(t, passthrough, 120*time.Millisecond, 0,
				`{"type":"response.output_text.delta","delta":"test output"}`)
			require.NotNil(t, result.firstTokenMs)
			require.GreaterOrEqual(t, *result.firstTokenMs, 100)
		})
	}
}

func TestOpenAIResponsesTTFTStartsAtCompletedImage(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			result := runSyntheticVisibleTTFTStream(t, passthrough, 120*time.Millisecond, 0,
				`{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"dGVzdA=="}}`)
			require.NotNil(t, result.firstTokenMs)
			require.GreaterOrEqual(t, *result.firstTokenMs, 100)
		})
	}
}

func TestOpenAINativeProgressDisarmsTimeoutWithoutStartingTTFT(t *testing.T) {
	result := runSyntheticVisibleTTFTStream(t, false, 1200*time.Millisecond, 1,
		`{"type":"response.output_text.delta","delta":"test output"}`)
	require.NotNil(t, result.firstTokenMs)
	require.GreaterOrEqual(t, *result.firstTokenMs, 1100)
}

func runSyntheticVisibleTTFTStream(t *testing.T, passthrough bool, visibleDelay time.Duration, timeoutSeconds int, visibleEvent string) *openaiStreamingResult {
	return runSyntheticTTFTStream(t, passthrough, true, visibleDelay, timeoutSeconds,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`, visibleEvent)
}

func runSyntheticTTFTStream(t *testing.T, passthrough, visibleOutputTTFT bool, visibleDelay time.Duration, timeoutSeconds int, preVisibleEvent, visibleEvent string) *openaiStreamingResult {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:                     defaultMaxLineSize,
		OpenAIFirstOutputTimeoutSeconds: timeoutSeconds,
	}}
	svc := &OpenAIGatewayService{cfg: cfg, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
	reader, writer := io.Pipe()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer func() { _ = writer.Close() }()
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\n")
		_, _ = io.WriteString(writer, "data: "+preVisibleEvent+"\n\n")
		time.Sleep(visibleDelay)
		_, _ = io.WriteString(writer, "data: "+visibleEvent+"\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	account := &Account{ID: 1, Platform: PlatformOpenAI}
	started := time.Now()

	var result *openaiStreamingResult
	var err error
	streamCtx := withOpenAIVisibleOutputTTFT(context.Background(), visibleOutputTTFT)
	if passthrough {
		var passResult *openaiStreamingResultPassthrough
		passResult, err = svc.handleStreamingResponsePassthrough(streamCtx, resp, c, account, started, "test-model", "test-model")
		if passResult != nil {
			result = &openaiStreamingResult{firstTokenMs: passResult.firstTokenMs}
		}
	} else {
		result, err = svc.handleStreamingResponse(streamCtx, resp, c, account, started, "test-model", "test-model")
	}
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), visibleEvent)
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("synthetic upstream writer did not exit")
	}
	return result
}
