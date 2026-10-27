package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSMessageTooBigResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, reason := range []string{"message_too_big", "prewarm_message_too_big"} {
		for _, keepalive := range []bool{false, true} {
			t.Run(reason+"/keepalive="+map[bool]string{false: "false", true: "true"}[keepalive], func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				if keepalive {
					declareOpenAIStreamResponseMetadataTrailers(c)
					c.Header("Content-Type", "text/event-stream")
					n, err := c.Writer.Write([]byte(":\n\n"))
					require.NoError(t, err)
					recordOpenAIStreamKeepaliveBytes(c, n)
					c.Writer.Flush()
				}
				wsErr := wrapOpenAIWSFallback(reason, errors.New("received close frame: StatusMessageTooBig; private diagnostic"))
				svc := &OpenAIGatewayService{}
				require.True(t, svc.writeOpenAIWSFallbackErrorResponse(c, nil, wsErr))
				require.True(t, IsResponseCommitted(c))
				require.NotContains(t, recorder.Body.String(), "private diagnostic")
				if keepalive {
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed"))
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if strings.HasPrefix(line, "data: ") {
							event := strings.TrimPrefix(line, "data: ")
							require.True(t, gjson.Valid(event))
							require.Equal(t, "upstream_error", gjson.Get(event, "response.error.code").String())
							require.Contains(t, gjson.Get(event, "response.error.message").String(), "message size limit exceeded (1009)")
						}
					}
				} else {
					require.Equal(t, http.StatusBadGateway, recorder.Code)
					require.True(t, gjson.ValidBytes(recorder.Body.Bytes()))
					require.Equal(t, "upstream_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					require.Contains(t, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String(), "message size limit exceeded (1009)")
				}
				before := recorder.Body.String()
				require.False(t, svc.writeOpenAIWSFallbackErrorResponse(c, nil, wsErr))
				require.Equal(t, before, recorder.Body.String(), "terminal response must only be written once")
			})
		}
	}
}

func TestOpenAIWSFallbackErrorResponsePreservesSemanticOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	const output = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"already delivered\"}\n\n"
	_, err := c.Writer.Write([]byte(output))
	require.NoError(t, err)
	c.Writer.Flush()
	wsErr := wrapOpenAIWSFallback("message_too_big", errors.New("received close 1009"))
	require.False(t, (&OpenAIGatewayService{}).writeOpenAIWSFallbackErrorResponse(c, nil, wsErr))
	require.Equal(t, output, recorder.Body.String())
}
