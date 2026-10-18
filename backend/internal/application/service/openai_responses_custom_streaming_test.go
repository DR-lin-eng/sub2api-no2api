package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesPassthroughPreservesNativeCustomTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","stream":%t,"tools":[{"type":"custom","name":"exec","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}],"tool_choice":{"type":"custom","name":"exec"},"input":[{"type":"custom_tool_call","id":"ctc_prev","call_id":"call_prev","name":"exec","input":"pwd"},{"type":"custom_tool_call_output","call_id":"call_prev","output":"/tmp"}]}`, stream))
			const output = `{"type":"custom_tool_call","id":"ctc_exec","call_id":"call_exec","name":"exec","input":"hello world","status":"completed"}`
			completed := `{"id":"resp_native","created_at":1,"output":[` + output + `],"usage":{"input_tokens":3,"output_tokens":4}}`
			upstreamBody := completed
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				upstreamBody = passthroughSSEData(`{"type":"response.output_item.added","sequence_number":0,"item":{"type":"custom_tool_call","id":"ctc_exec","call_id":"call_exec","name":"exec","status":"in_progress"}}`) +
					passthroughSSEData(`{"type":"response.custom_tool_call_input.delta","sequence_number":1,"item_id":"ctc_exec","delta":"hello"}`) +
					passthroughSSEData(`{"type":"response.custom_tool_call_input.delta","sequence_number":2,"item_id":"ctc_exec","delta":" world"}`) +
					passthroughSSEData(`{"type":"response.custom_tool_call_input.done","sequence_number":3,"item_id":"ctc_exec","input":"hello world"}`) +
					passthroughSSEData(`{"type":"response.output_item.done","sequence_number":4,"item":`+output+`}`) +
					passthroughSSEData(`{"type":"response.completed","sequence_number":5,"response":`+completed+`}`)
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}}
			account := passthroughArgsFallbackAccount()
			account.Extra = map[string]any{"openai_passthrough": true,
				openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses)}
			svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig(), httpUpstream: upstream}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 1)
			for _, field := range []string{"tools", "tool_choice"} {
				require.JSONEq(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(upstream.lastBody, field).Raw)
			}
			// Existing replay normalization removes historical item IDs while
			// keeping native custom types, call IDs, and freeform input intact.
			require.JSONEq(t, `[{"type":"custom_tool_call","call_id":"call_prev","name":"exec","input":"pwd"},{"type":"custom_tool_call_output","call_id":"call_prev","output":"/tmp"}]`, gjson.GetBytes(upstream.lastBody, "input").Raw)
			if stream {
				events := collectSSEDataPayloads(t, recorder.Body.String())
				require.Len(t, events, 6)
				require.Equal(t, "hello", gjson.Get(events[1], "delta").String())
				require.Equal(t, " world", gjson.Get(events[2], "delta").String())
				require.Equal(t, "custom_tool_call", gjson.Get(events[5], "response.output.0.type").String())
			} else {
				require.Equal(t, "custom_tool_call", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
				require.Equal(t, "hello world", gjson.GetBytes(recorder.Body.Bytes(), "output.0.input").String())
			}
			t.Logf("native custom preserved: stream=%t requests=%d", stream, len(upstream.requests))
		})
	}
}

func TestGrokResponsesCustomToolDeltaBeforeUpstreamDone(t *testing.T) {
	source, writer := io.Pipe()
	body := newGrokResponsesClientToolStreamBody(source, apicompat.ResponsesClientToolMapping{
		CustomTools: map[string]bool{"exec": true},
	}, defaultMaxLineSize)
	defer func() { _ = body.Close() }()
	defer func() { _ = writer.Close() }()
	frames := make(chan string, 16)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				frames <- strings.TrimPrefix(scanner.Text(), "data: ")
			}
		}
	}()
	writeFrame := func(payload string) {
		t.Helper()
		_, err := io.WriteString(writer, "event: "+gjson.Get(payload, "type").String()+"\n"+passthroughSSEData(payload))
		require.NoError(t, err)
	}
	readFrame := func() string {
		t.Helper()
		select {
		case frame, ok := <-frames:
			require.True(t, ok, "upstream must remain open")
			return frame
		case <-time.After(3 * time.Second):
			t.Fatal("custom input delta was buffered while the upstream waited to send done")
			return ""
		}
	}
	writeFrame(`{"type":"response.output_item.added","sequence_number":10,"item":{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec"}}`)
	require.Equal(t, "custom_tool_call", gjson.Get(readFrame(), "item.type").String())
	for i, fragment := range []string{`{"input":"hello`, ` world`} {
		writeFrame(functionArgsDeltaJSON(0, "fc_exec", "call_exec", "exec", fragment))
		frame := readFrame()
		require.Equal(t, "response.custom_tool_call_input.delta", gjson.Get(frame, "type").String())
		require.Equal(t, "ctc_exec", gjson.Get(frame, "item_id").String())
		require.Equal(t, 11+i, int(gjson.Get(frame, "sequence_number").Int()))
		require.Equal(t, []string{"hello", " world"}[i], gjson.Get(frame, "delta").String())
	}
	t.Log("before upstream done: received hello world")
	writeFrame(functionArgsDoneJSON(0, "fc_exec", "call_exec", "exec", `{"input":"hello world"}`))
	done := readFrame()
	require.Equal(t, "response.custom_tool_call_input.done", gjson.Get(done, "type").String())
	require.Equal(t, "hello world", gjson.Get(done, "input").String())
	require.Equal(t, 13, int(gjson.Get(done, "sequence_number").Int()))
}
