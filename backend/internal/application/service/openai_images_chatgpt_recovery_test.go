//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatGPTAppImagesResumeDoesNotCreateAnotherTurn(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"assistant\"},\"metadata\":{\"image_gen_async\":true}}}\n\n")
		case "/backend-api/f/conversation/resume":
			body, _ := io.ReadAll(r.Body)
			require.Equal(t, "conv-fixture", gjson.GetBytes(body, "conversation_id").String())
			require.Positive(t, gjson.GetBytes(body, "offset").Int())
			chatGPTAppImageStream(w, encoded)
		default:
			t.Errorf("unexpected recovery path %s", r.URL.Path)
		}
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	calls := upstream.imageCalls()
	require.Len(t, calls, 2)
	require.Equal(t, "/backend-api/f/conversation/resume", calls[1].path)
}

func TestChatGPTAppImagesAsyncHistoryRecovery(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/f/conversation" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"assistant\"},\"metadata\":{\"image_gen_async\":true}}}\n\ndata: [DONE]\n\n")
			return
		}
		require.Equal(t, "/backend-api/conversation/conv-fixture", r.URL.Path)
		fmt.Fprintf(w, `{"conversation_id":"conv-fixture","mapping":{"completed":{"message":{"author":{"role":"assistant"},"output":[{"type":"image_generation_call","status":"completed","result":%q}]}}}}`, encoded)
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.imageCalls(), 2)
}

type chatGPTAppTopicTestConn struct {
	frames [][]byte
	writes []any
	closed bool
}

func (c *chatGPTAppTopicTestConn) WriteJSON(_ context.Context, v any) error {
	c.writes = append(c.writes, v)
	return nil
}
func (c *chatGPTAppTopicTestConn) ReadMessage(context.Context) ([]byte, error) {
	if len(c.frames) == 0 {
		return nil, io.EOF
	}
	b := c.frames[0]
	c.frames = c.frames[1:]
	return b, nil
}
func (c *chatGPTAppTopicTestConn) Ping(context.Context) error { return nil }
func (c *chatGPTAppTopicTestConn) Close() error               { c.closed = true; return nil }

type chatGPTAppTopicTestDialer struct {
	conn    *chatGPTAppTopicTestConn
	headers http.Header
}

func (d *chatGPTAppTopicTestDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.headers = headers.Clone()
	return d.conn, 101, http.Header{}, nil
}

func TestChatGPTAppImagesTopicHistoryNullableIDsAndEncodedSSE(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	topic := "conversation-conv-fixture"
	streamData := fmt.Sprintf("event: delta\ndata: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"assistant\"},\"output\":[{\"type\":\"image_generation_call\",\"status\":\"completed\",\"result\":%q}]}}\n\n", encoded)
	item := map[string]any{"type": "message", "topic_id": topic, "payload": map[string]any{"type": "conversation-turn-stream", "payload": map[string]any{"type": "stream-item", "conversation_id": "conv-fixture", "turn_id": "turn", "stream_item_id": nil, "parent_stream_item_id": nil, "encoded_item": streamData}}}
	done := map[string]any{"type": "message", "topic_id": topic, "payload": map[string]any{"type": "conversation-turn-stream", "payload": map[string]any{"type": "done", "conversation_id": "conv-fixture", "turn_id": "turn"}}}
	frame, err := json.Marshal([]any{map[string]any{"id": 1, "reply": map[string]any{"type": "connect"}}, map[string]any{"id": 2, "reply": map[string]any{"type": "subscribe", "topic_id": topic, "recovered": true, "last_offset": "2", "catchups": []any{item, item, done}}}})
	require.NoError(t, err)
	dialer := &chatGPTAppTopicTestDialer{conn: &chatGPTAppTopicTestConn{frames: [][]byte{frame}}}
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\"}\n\ndata: {\"type\":\"stream_handoff\",\"options\":[{\"type\":\"subscribe_ws_topic\",\"topic_id\":\"conversation-conv-fixture\"}]}\n\ndata: [DONE]\n\n")
		case "/backend-api/celsius/ws/user":
			fmt.Fprint(w, `{"websocket_url":"wss://chatgpt.com/ws-fixture"}`)
		default:
			t.Errorf("unexpected topic recovery path %s", r.URL.Path)
		}
	})
	svc.openaiWSPassthroughDialer = dialer
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.imageCalls(), 2)
	require.Empty(t, dialer.headers.Get("Authorization"))
	require.Empty(t, dialer.headers.Get("ChatGPT-Account-ID"))
	require.True(t, dialer.conn.closed)
	command, err := json.Marshal(dialer.conn.writes[1])
	require.NoError(t, err)
	require.Equal(t, "0", gjson.GetBytes(command, "0.command.offset").String())
	require.False(t, gjson.GetBytes(command, "0.command.include_all_history").Exists())
}

func TestChatGPTAppImagesInvalidBrowserChallengeDoesNotGenerate(t *testing.T) {
	for _, body := range []string{`{"turnstile":{"required":true,"dx":"untrusted-code"}}`, `{"proofofwork":{"required":true,"difficulty":"00"}}`, `{"turnstile":{"required":true}}`} {
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		request := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
		c := newOpenAIImagesContext(t, request)
		parsed, err := svc.ParseOpenAIImagesRequest(c, request)
		require.NoError(t, err)
		result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), request, parsed, "")
		require.Nil(t, result)
		require.ErrorContains(t, err, "chatgpt_verification_required")
		require.Len(t, upstream.requests, 1)
		require.Equal(t, "/backend-api/sentinel/chat-requirements/prepare", upstream.lastReq.URL.Path)
		require.Equal(t, 403, c.Writer.Status())
	}
}

func TestChatGPTAppImagesModeIsFixedForTheCurrentRequest(t *testing.T) {
	svc := &OpenAIGatewayService{settingService: &SettingService{}}
	c := newOpenAIImagesContext(t, []byte(`{}`))
	account := chatGPTAppImageTestAccount()
	require.True(t, svc.useChatGPTAppImages(t.Context(), c, account))
	svc.settingService.refreshOpenAIImagesMode(true)
	require.True(t, svc.useChatGPTAppImages(t.Context(), c, account))
	require.False(t, svc.useChatGPTAppImages(t.Context(), newOpenAIImagesContext(t, []byte(`{}`)), account))
}

func TestChatGPTAppImagesRelayBaseNormalization(t *testing.T) {
	for _, raw := range []string{"https://relay.example/backend-api/codex", "https://relay.example/backend-api/codex/responses", "https://relay.example/backend-api"} {
		base, err := chatGPTImageBackendBase(raw + "?relay_key=fixture")
		require.NoError(t, err)
		session := &chatGPTImageSession{base: base}
		endpoint, err := session.endpoint("/files/download/file_fixture?inline=true")
		require.NoError(t, err)
		require.Equal(t, "https://relay.example/backend-api/files/download/file_fixture?inline=true&relay_key=fixture", endpoint)
	}
}
