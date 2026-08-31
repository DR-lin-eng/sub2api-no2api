//go:build unit

package service

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkOpenAIWSV2PassthroughCyberPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"blocked by upstream policy"},"usage":{"input_tokens":17,"output_tokens":3}}}`)

	require.True(t, markOpenAIWSV2PassthroughCyberPolicy(c, payload))
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, "cyber_policy", mark.Code)
	require.Equal(t, 17, mark.UpstreamInTok)
	require.Equal(t, 3, mark.UpstreamOutTok)
}

func TestMarkOpenAIWSV2PassthroughCyberPolicyIgnoresOrdinaryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	payload := []byte(`{"type":"response.failed","response":{"error":{"type":"upstream_error","message":"temporarily unavailable"}}}`)

	require.False(t, markOpenAIWSV2PassthroughCyberPolicy(c, payload))
	require.Nil(t, GetOpsCyberPolicy(c))
}
