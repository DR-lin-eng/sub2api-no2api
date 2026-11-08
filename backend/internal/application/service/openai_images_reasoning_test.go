//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Responses inherits the outer reasoning effort as image complexity when the
// tool omits complexity. Earlier image models reject that inherited option.
func legacyImageComplexityUpstream(t *testing.T, model string) *codexModelsHTTPUpstreamStub {
	t.Helper()
	return &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		require.Equal(t, "/backend-api/codex/responses", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if req.Header.Get("Content-Encoding") == "zstd" {
			decoder, err := zstd.NewReader(nil)
			require.NoError(t, err)
			defer decoder.Close()
			body, err = decoder.DecodeAll(body, nil)
			require.NoError(t, err)
		}
		require.True(t, gjson.ValidBytes(body))
		require.Equal(t, model, gjson.GetBytes(body, "tools.0.model").String())
		complexity := gjson.GetBytes(body, "tools.0.complexity").String()
		if complexity == "" {
			complexity = gjson.GetBytes(body, "reasoning.effort").String()
		}
		if complexity != "" {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"type":"invalid_request_error","message":"svgComplexity '%s' is not supported for this image model."}}`, complexity))),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000000,\"tool_usage\":{\"image_gen\":{\"input_tokens\":10,\"output_tokens\":20,\"output_tokens_details\":{\"image_tokens\":20},\"images\":1}},\"output\":[{\"type\":\"image_generation_call\",\"result\":\"AA==\",\"output_format\":\"png\"}]}}\n\ndata: [DONE]\n\n")),
		}, nil
	}}
}

func TestOpenAIImagesLegacyReasoningAccountTest(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		t.Run(model, func(t *testing.T) {
			c, rec := newOpenAIImagesTestContext(t, nil)
			svc := &AccountTestService{httpUpstream: legacyImageComplexityUpstream(t, model)}
			err := svc.testOpenAIImageOAuth(c, context.Background(), directImagesTestAccount(), model, "draw a cat")
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.Contains(t, rec.Body.String(), "data:image/png;base64,AA==")
		})
	}
}

func TestOpenAIImagesLegacyReasoningAcrossImageEndpoints(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		for _, format := range []string{"generation", "json-edit", "multipart-edit"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", model, format, stream), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw a cat","stream":%t`, model, stream))
					if format == "json-edit" {
						body = append(body, []byte(`,"images":[{"image_url":"data:image/png;base64,AA=="}]`)...)
					}
					body = append(body, '}')
					contentType := "application/json"
					if format == "multipart-edit" {
						var buf bytes.Buffer
						writer := multipart.NewWriter(&buf)
						for key, value := range map[string]string{"model": model, "prompt": "draw a cat", "stream": fmt.Sprint(stream)} {
							require.NoError(t, writer.WriteField(key, value))
						}
						part, err := writer.CreateFormFile("image", "input.png")
						require.NoError(t, err)
						_, err = part.Write([]byte("image-bytes"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						body, contentType = buf.Bytes(), writer.FormDataContentType()
					}
					c, rec := newOpenAIImagesTestContext(t, body)
					c.Request.Header.Set("Content-Type", contentType)
					if format != "generation" {
						c.Request.URL.Path = openAIImagesEditsEndpoint
					}
					svc := newOpenAIImagesService(legacyImageComplexityUpstream(t, model))
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					result, err := forwardLegacyOpenAIImagesForTest(svc, context.Background(), c, directImagesTestAccount(), body, parsed, "")
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Equal(t, stream, result.Stream)
					require.Equal(t, model, result.UpstreamModel)
					require.Equal(t, 1, result.ImageCount)
					require.Equal(t, 20, result.Usage.ImageOutputTokens)
					if stream {
						event := "image_generation.completed"
						if format != "generation" {
							event = "image_edit.completed"
						}
						require.Contains(t, rec.Body.String(), "event: "+event)
					} else {
						require.Equal(t, "AA==", gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
					}
				})
			}
		}
	}
}
