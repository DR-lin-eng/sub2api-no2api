//go:build unit

package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConvertClaudeMessagesToGeminiGenerateContent_ExtractsToolResultImages(t *testing.T) {
	claudeReq := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_123",
						"name": "inspect",
						"input": map[string]any{
							"path": "image.png",
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_123",
						"content": []any{
							map[string]any{"type": "text", "text": "found it"},
							map[string]any{
								"type": "image",
								"source": map[string]any{
									"type":       "base64",
									"media_type": "image/png",
									"data":       "aGVsbG8=",
								},
							},
						},
					},
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_123",
						"content": []any{
							map[string]any{
								"type": "image",
								"source": map[string]any{
									"type":       "base64",
									"media_type": "image/jpeg",
									"data":       "d29ybGQ=",
								},
							},
						},
					},
				},
			},
		},
	}
	body, err := json.Marshal(claudeReq)
	require.NoError(t, err)

	converted, err := convertClaudeMessagesToGeminiGenerateContent(body)
	require.NoError(t, err)

	var geminiReq map[string]any
	require.NoError(t, json.Unmarshal(converted, &geminiReq))
	contents, ok := geminiReq["contents"].([]any)
	require.True(t, ok)
	content, ok := contents[1].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 4)

	firstPart, ok := parts[0].(map[string]any)
	require.True(t, ok)
	firstResponse, ok := firstPart["functionResponse"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "inspect", firstResponse["name"])
	firstResponseBody, ok := firstResponse["response"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "found it", firstResponseBody["content"])

	secondPart, ok := parts[1].(map[string]any)
	require.True(t, ok)
	secondResponse, ok := secondPart["functionResponse"].(map[string]any)
	require.True(t, ok)
	secondResponseBody, ok := secondResponse["response"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[image attached below]", secondResponseBody["content"])

	firstImagePart, ok := parts[2].(map[string]any)
	require.True(t, ok)
	firstImage, ok := firstImagePart["inlineData"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image/png", firstImage["mimeType"])
	require.Equal(t, "aGVsbG8=", firstImage["data"])
	secondImagePart, ok := parts[3].(map[string]any)
	require.True(t, ok)
	secondImage, ok := secondImagePart["inlineData"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image/jpeg", secondImage["mimeType"])
	require.Equal(t, "d29ybGQ=", secondImage["data"])
}
