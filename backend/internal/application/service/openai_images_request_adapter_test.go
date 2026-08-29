package service

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

type customImageModelResolverStub struct {
	capabilities map[string]bool
	adapters     map[string]map[string]any
}

func (s customImageModelResolverStub) HasCapability(_ context.Context, model, capability string) (bool, error) {
	return capability == "image" && s.capabilities[model], nil
}

func (s customImageModelResolverStub) ResolveVideoAPIType(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (s customImageModelResolverStub) ResolveRequestAdapter(
	_ context.Context,
	model string,
) (map[string]any, bool, error) {
	adapter, ok := s.adapters[model]
	return adapter, ok, nil
}

func TestCustomImageModelMappingUsesClientConfigAndAdapter(t *testing.T) {
	clientAdapter := map[string]any{"version": 1, "body": map[string]any{"mode": "off"}}
	svc := &OpenAIGatewayService{customModelCapabilities: customImageModelResolverStub{
		capabilities: map[string]bool{"public-image": true},
		adapters:     map[string]map[string]any{"public-image": clientAdapter},
	}}

	require.NoError(t, svc.validateOpenAIImagesMappedModel(
		context.Background(), "vendor-model-v2", "public-image", "channel-image",
	))
	adapter, configured, err := svc.resolveCustomModelRequestAdapter(
		context.Background(), "public-image", "channel-image", "vendor-model-v2",
	)
	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, clientAdapter, adapter)
}

func TestValidateCustomModelRequestAdapterRejectsUnsafeAndMalformedRules(t *testing.T) {
	valid := map[string]any{
		"version": 1,
		"upstream": map[string]any{
			"path":         "/v1/images/generations",
			"content_type": "application/json",
		},
		"headers": map[string]any{
			"set": map[string]any{"x-provider-mode": "{{request.model}}"},
		},
		"body": map[string]any{
			"mode":  "merge",
			"value": map[string]any{"image": "{{request.input_images}}"},
		},
	}
	require.NoError(t, validateCustomModelRequestAdapter(valid))

	for name, adapter := range map[string]map[string]any{
		"absolute URL": {
			"upstream": map[string]any{"path": "https://internal.example/upload"},
		},
		"authorization header": {
			"headers": map[string]any{"set": map[string]any{"Authorization": "secret"}},
		},
		"malformed variable": {
			"body": map[string]any{"mode": "merge", "value": map[string]any{"prompt": "{{token}}"}},
		},
		"unsupported mode": {
			"body": map[string]any{"mode": "append"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateCustomModelRequestAdapter(adapter)
			require.ErrorIs(t, err, ErrCustomModelConfigInvalid)
		})
	}
}

func TestApplyOpenAIImagesRequestAdapterMergesJSONAndRewritesEndpoint(t *testing.T) {
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesEditsEndpoint}
	body := []byte(`{"model":"image-model","prompt":"cat","response_format":"url","extra":{"a":1}}`)
	adapter := map[string]any{
		"version": 1,
		"match": map[string]any{
			"endpoint": openAIImagesEditsEndpoint,
		},
		"upstream": map[string]any{
			"path":         openAIImagesGenerationsEndpoint,
			"content_type": "application/json",
		},
		"headers": map[string]any{
			"set": map[string]any{"x-provider-mode": "image"},
		},
		"body": map[string]any{
			"mode": "merge",
			"value": map[string]any{
				"response_format": nil,
				"extra":           map[string]any{"b": 2},
			},
		},
	}

	adapted, err := applyOpenAIImagesRequestAdapter(body, "application/json", parsed, "upstream-image-model", adapter)
	require.NoError(t, err)
	require.Equal(t, openAIImagesGenerationsEndpoint, adapted.Endpoint)
	require.Equal(t, "application/json", adapted.ContentType)
	require.Equal(t, "image", adapted.Headers["x-provider-mode"])

	var payload map[string]any
	require.NoError(t, json.Unmarshal(adapted.Body, &payload))
	require.NotContains(t, payload, "response_format")
	require.Equal(t, "cat", payload["prompt"])
	require.Equal(t, map[string]any{"a": float64(1), "b": float64(2)}, payload["extra"])
}

func TestApplyOpenAIImagesRequestAdapterConvertsMultipartFilesToDataURIArray(t *testing.T) {
	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.WriteField("model", "image-model"))
	require.NoError(t, writer.WriteField("prompt", "cat"))
	require.NoError(t, writer.WriteField("n", "2"))
	file, err := writer.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("\x89PNG\r\n\x1a\nimage"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	adapter := map[string]any{
		"version": 1,
		"match": map[string]any{
			"endpoint": openAIImagesEditsEndpoint,
		},
		"upstream": map[string]any{
			"path":         openAIImagesGenerationsEndpoint,
			"content_type": "application/json",
		},
		"body": map[string]any{
			"mode":  "off",
			"value": map[string]any{},
		},
	}

	adapted, err := applyOpenAIImagesRequestAdapter(
		input.Bytes(),
		writer.FormDataContentType(),
		&OpenAIImagesRequest{Endpoint: openAIImagesEditsEndpoint},
		"upstream-image-model",
		adapter,
	)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(adapted.Body, &payload))
	require.Equal(t, "image-model", payload["model"])
	require.Equal(t, "cat", payload["prompt"])
	require.Equal(t, float64(2), payload["n"])
	images, ok := payload["image"].([]any)
	require.True(t, ok)
	require.Len(t, images, 1)
	require.Contains(t, images[0], "data:image/png;base64,")
}

func TestApplyOpenAIImagesRequestAdapterRendersTypedVariables(t *testing.T) {
	body := []byte(`{"model":"public-model","prompt":"draw a cat","metadata":{"tenant":"demo"}}`)
	adapter := map[string]any{
		"upstream": map[string]any{
			"content_type": "application/json",
		},
		"body": map[string]any{
			"mode": "replace",
			"value": map[string]any{
				"model":   "{{request.upstream_model}}",
				"prompt":  "{{request.prompt}}",
				"count":   "{{request.n}}",
				"enabled": "{{request.stream}}",
				"tenant":  "{{request.body.metadata.tenant}}",
			},
		},
	}

	adapted, err := applyOpenAIImagesRequestAdapter(
		body,
		"application/json",
		&OpenAIImagesRequest{
			Endpoint: openAIImagesGenerationsEndpoint,
			Model:    "public-model",
			Prompt:   "draw a cat",
			N:        3,
			Stream:   true,
		},
		"upstream-model",
		adapter,
	)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(adapted.Body, &payload))
	require.Equal(t, "upstream-model", payload["model"])
	require.Equal(t, "draw a cat", payload["prompt"])
	require.Equal(t, float64(3), payload["count"])
	require.Equal(t, true, payload["enabled"])
	require.Equal(t, "demo", payload["tenant"])
}
