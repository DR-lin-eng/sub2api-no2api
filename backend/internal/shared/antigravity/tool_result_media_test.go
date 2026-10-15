//go:build unit

package antigravity

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransformClaudeToGemini_ToolResultImagesReachUpstream(t *testing.T) {
	claudeReq := &ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Messages: []ClaudeMessage{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_img_1","name":"read_image","input":{"path":"colors.png"}}
				]`),
			},
			{
				Role: "user",
				Content: json.RawMessage(`[
					{
						"type":"tool_result",
						"tool_use_id":"toolu_img_1",
						"content":[
							{"type":"text","text":"loaded colors.png"},
							{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}
						]
					},
					{
						"type":"tool_result",
						"tool_use_id":"toolu_img_1",
						"content":[
							{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"d29ybGQ="}}
						]
					}
				]`),
			},
		},
	}

	body, err := TransformClaudeToGeminiWithOptions(claudeReq, "project-1", "claude-sonnet-4-5", DefaultTransformOptions())
	require.NoError(t, err)

	var req V1InternalRequest
	require.NoError(t, json.Unmarshal(body, &req))
	require.Len(t, req.Request.Contents, 2)

	userParts := req.Request.Contents[1].Parts
	require.Len(t, userParts, 4)

	require.NotNil(t, userParts[0].FunctionResponse)
	require.Equal(t, "read_image", userParts[0].FunctionResponse.Name)
	require.Equal(t, "toolu_img_1", userParts[0].FunctionResponse.ID)
	require.Equal(t, "loaded colors.png", userParts[0].FunctionResponse.Response["result"])

	require.NotNil(t, userParts[1].InlineData)
	require.Equal(t, "image/png", userParts[1].InlineData.MimeType)
	require.Equal(t, "aGVsbG8=", userParts[1].InlineData.Data)

	require.NotNil(t, userParts[2].FunctionResponse)
	require.Equal(t, "read_image", userParts[2].FunctionResponse.Name)
	require.Equal(t, "Command executed successfully.", userParts[2].FunctionResponse.Response["result"])

	require.NotNil(t, userParts[3].InlineData)
	require.Equal(t, "image/jpeg", userParts[3].InlineData.MimeType)
	require.Equal(t, "d29ybGQ=", userParts[3].InlineData.Data)
}
