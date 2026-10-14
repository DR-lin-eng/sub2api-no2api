package service

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleChatStreamingResponseCommitsHeadersBeforeFirstChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax} {
		for _, mode := range []struct {
			name          string
			buffered      bool
			async         bool
			filterHeaders bool
		}{
			{name: "immediate"},
			{name: "immediate_async_filtered", async: true, filterHeaders: true},
			{name: "pending", buffered: true},
			{name: "pending_async_filtered", buffered: true, async: true, filterHeaders: true},
		} {
			t.Run(platform+"/"+mode.name, func(t *testing.T) {
				cfg := &config.Config{}
				if mode.async {
					cfg.Gateway.StreamDataIntervalTimeout = 10
				}
				svc := &OpenAIGatewayService{cfg: cfg}
				if mode.filterHeaders {
					svc.responseHeaderFilter = compileResponseHeaderFilter(cfg)
				}
				reader, writer := io.Pipe()
				t.Cleanup(func() { _ = reader.Close() })
				t.Cleanup(func() { _ = writer.Close() })
				releaseTerminal := make(chan struct{})
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(releaseTerminal) }) }
				defer release()
				go func() {
					defer func() { _ = writer.Close() }()
					_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_headers\",\"model\":\"test-model\",\"status\":\"in_progress\",\"output\":[]}}\n\n")
					_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first chunk\"}\n\n")
					<-releaseTerminal
					_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_headers\",\"model\":\"test-model\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":11,\"output_tokens\":5,\"total_tokens\":16}}}\n\n")
				}()
				type outcome struct {
					result *OpenAIForwardResult
					err    error
				}
				finished := make(chan outcome, 1)
				upstream := &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
						"X-Request-Id": []string{"headers-test"},
					},
					Body: reader,
				}
				router := gin.New()
				router.GET("/stream", func(c *gin.Context) {
					bodyLen := 0
					if mode.buffered {
						bodyLen = openAISilentRefusalMinRequestBodyBytes
					}
					result, err := svc.handleChatStreamingResponse(upstream, c,
						&Account{ID: 1, Platform: platform}, "test-model", "test-model", "test-model", time.Now(), bodyLen)
					finished <- outcome{result: result, err: err}
				})
				server := httptest.NewServer(router)
				defer func() {
					release()
					server.Close()
				}()
				client := server.Client()
				client.Timeout = 2 * time.Second
				response, err := client.Get(server.URL + "/stream")
				require.NoError(t, err, "SSE headers must arrive before the upstream terminal event")
				defer func() { _ = response.Body.Close() }()
				require.Equal(t, http.StatusOK, response.StatusCode)
				require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
				require.Equal(t, "no-cache", response.Header.Get("Cache-Control"))
				require.Equal(t, "no", response.Header.Get("X-Accel-Buffering"))
				if mode.filterHeaders {
					require.Equal(t, "headers-test", response.Header.Get("X-Request-Id"))
				}
				scanner := bufio.NewScanner(response.Body)
				var output strings.Builder
				for scanner.Scan() {
					_, _ = output.WriteString(scanner.Text() + "\n")
					if strings.Contains(scanner.Text(), "first chunk") {
						break
					}
				}
				require.NoError(t, scanner.Err())
				require.Contains(t, output.String(), "first chunk", "small chunks must be flushed before completion")
				select {
				case <-finished:
					t.Fatal("forwarder completed before the terminal event was released")
				default:
				}
				release()
				for scanner.Scan() {
					_, _ = output.WriteString(scanner.Text() + "\n")
				}
				require.NoError(t, scanner.Err())
				require.Equal(t, 1, strings.Count(output.String(), "data: [DONE]"))
				require.Contains(t, output.String(), `"completion_tokens":5`)
				got := <-finished
				require.NoError(t, got.err)
				require.NotNil(t, got.result)
				require.Equal(t, 11, got.result.Usage.InputTokens)
				require.Equal(t, 5, got.result.Usage.OutputTokens)
			})
		}
	}
}
