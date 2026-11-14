//go:build unit

package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatGPTAppPNGToGIFEditsWithExplicitDefaults(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		for _, input := range []string{"json", "multipart/image", "multipart/image[]"} {
			t.Run(model+"/"+input, func(t *testing.T) {
				png := encodeOpenAIImageTestPNG(t, 3, 2)
				pngBytes, err := base64.StdEncoding.DecodeString(png)
				require.NoError(t, err)
				gif := image26AnimatedGIF(t, 4)
				svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/backend-api/files":
						fmt.Fprint(w, `{"file_id":"file_reference","upload_url":"https://signed.oaiusercontent.com/reference.png?sig=fixture","upload_headers":{"Authorization":"must-not-forward"}}`)
					case "/reference.png":
						require.Equal(t, http.MethodPut, r.Method)
						require.Empty(t, r.Header.Get("Authorization"))
						require.Empty(t, r.Header.Get("Cookie"))
						require.Empty(t, r.Header.Get("ChatGPT-Account-ID"))
						require.Equal(t, "BlockBlob", r.Header.Get("x-ms-blob-type"))
						require.Equal(t, "2020-04-08", r.Header.Get("x-ms-version"))
						require.Equal(t, "image/png", r.Header.Get("x-ms-blob-content-type"))
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						require.Equal(t, pngBytes, body)
						w.WriteHeader(http.StatusCreated)
					case "/backend-api/files/process_upload_stream":
						w.Header().Set("Content-Type", "application/x-ndjson")
						fmt.Fprint(w, "{\"file_id\":\"file_reference\",\"event\":\"file.processing.completed\"}\n")
					case "/backend-api/f/conversation":
						chatGPTAppImageStream(w, "data:image/gif;base64,"+gif)
					default:
						t.Errorf("unexpected upstream path: %s", r.URL.Path)
						http.Error(w, `{"detail":"GIF output is not enabled for this request."}`, 400)
					}
				})
				body := fmt.Appendf(nil, `{"model":%q,"prompt":"animate my PNG","images":[{"image_url":"data:image/png;base64,%s"}],"output_format":"gif","frame_count":4,"frame_duration_ms":200,"size":"auto","quality":"auto","complexity":"high","moderation":"auto"}`, model, png)
				contentType := "application/json"
				if input != "json" {
					var buf bytes.Buffer
					writer := multipart.NewWriter(&buf)
					for key, value := range map[string]string{"model": model, "prompt": "animate my PNG", "output_format": "gif", "frame_count": "4", "frame_duration_ms": "200", "size": "auto", "quality": "auto", "complexity": "high", "moderation": "auto"} {
						require.NoError(t, writer.WriteField(key, value))
					}
					field := "image"
					if input == "multipart/image[]" {
						field = "image[]"
					}
					part, err := writer.CreateFormFile(field, "reference.png")
					require.NoError(t, err)
					_, err = part.Write(pngBytes)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					body, contentType = buf.Bytes(), writer.FormDataContentType()
				}
				c, rec := newOpenAIImagesTestContext(t, body)
				c.Request.URL.Path = openAIImagesEditsEndpoint
				c.Request.Header.Set("Content-Type", contentType)
				c.Set("api_key", &APIKey{Group: &Group{ImageCountBilling: true}})
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				require.False(t, openAIImagesRequiresNativeProtocol(parsed), "explicit defaults must retain App GIF support")
				result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
				require.NoError(t, err)
				require.Equal(t, 4, result.ImageCount)
				require.Len(t, result.ImageOutputSizes, 4)
				require.Equal(t, gif, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
				calls := upstream.imageCalls()
				require.Len(t, calls, 4)
				require.True(t, calls[1].public)
				require.True(t, calls[1].upload)
				require.True(t, calls[1].redirectsDisabled)
				conversation := gjson.ParseBytes(calls[3].body)
				require.Equal(t, "sediment://file_reference", conversation.Get("messages.0.content.parts.0.asset_pointer").String())
				require.EqualValues(t, 3, conversation.Get("messages.0.content.parts.0.width").Int())
				prompt := conversation.Get("messages.0.content.parts.1").String()
				require.Contains(t, prompt, "Animate the supplied reference images")
				require.Contains(t, prompt, "high planning complexity")
				require.Contains(t, prompt, "exactly 4 frames and 200 milliseconds")
				require.Contains(t, prompt, "animate my PNG")
				for _, key := range []string{"complexity", "quality", "output_format", "frame_count", "animation", "requestedMimeType"} {
					require.False(t, conversation.Get(key).Exists(), "do not invent App native parameters")
				}
				require.NotContains(t, rec.Body.String(), "fallback")
				require.NotContains(t, rec.Body.String(), "legacy")
			})
		}
	}
}

func TestChatGPTAppGIFNonDefaultControlsKeepNativeProtocol(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw","output_format":"svg","svg":{"model":"gpt-6-astra"}}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := newOpenAIImagesService(nil).ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.True(t, openAIImagesRequiresNativeProtocol(parsed))
}

func TestChatGPTAppOrdinaryImagesIgnoreOptionalNativeControls(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw a blue circle","size":"1536x1024","quality":"low","complexity":"medium","background":"transparent","output_format":"jpeg","output_compression":80,"partial_images":1,"moderation":"low","input_fidelity":"high","safety_identifier":"hashed","user":"legacy"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	encoded := encodeOpenAIImageTestPNG(t, 3, 2)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
		chatGPTAppImageStream(w, encoded)
	})
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.False(t, openAIImagesRequiresNativeProtocol(parsed))
	_, err = svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Len(t, upstream.imageCalls(), 1)
	require.Contains(t, rec.Body.String(), `"b64_json":"`+encoded+`"`)
	require.NotContains(t, rec.Body.String(), "legacy")
	require.NotContains(t, rec.Body.String(), "fallback")
}

func TestChatGPTAppGIFStandardClientControlsStayOnAppPath(t *testing.T) {
	for _, tc := range []struct{ extra, phrase string }{
		{`"complexity":"low"`, "low planning complexity"},
		{`"complexity":"medium"`, "medium planning complexity"},
		{`"quality":"low"`, "low output quality"},
		{`"quality":"high"`, "high output quality"},
		{`"moderation":"low"`, "low moderation setting"},
		{`"input_fidelity":"high"`, "high input fidelity"},
	} {
		t.Run(tc.extra, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"animate the reference","images":[{"image_url":"data:image/png;base64,AA=="}],"output_format":"gif",` + tc.extra + `}`)
			c := newOpenAIImagesContext(t, body)
			c.Request.URL.Path = openAIImagesEditsEndpoint
			parsed, err := newOpenAIImagesService(nil).ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			require.False(t, openAIImagesRequiresNativeProtocol(parsed), tc.extra)
			prompt := chatGPTAppImagePrompt(parsed, parsed.Prompt)
			require.Contains(t, prompt, "Animate the supplied reference images")
			require.Contains(t, prompt, tc.phrase)
			require.NotContains(t, prompt, strings.Trim(tc.extra, `"`))
		})
	}
}

func TestChatGPTAppPNGToGIFCurlControlsDoNotSelectNativeEndpoint(t *testing.T) {
	png := encodeOpenAIImageTestPNG(t, 3, 2)
	gif := image26AnimatedGIF(t, 9)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/files":
			fmt.Fprint(w, `{"file_id":"file_reference","upload_url":"https://signed.oaiusercontent.com/reference.png?sig=fixture","upload_headers":{}}`)
		case "/reference.png":
			w.WriteHeader(http.StatusCreated)
		case "/backend-api/files/process_upload_stream":
			w.Header().Set("Content-Type", "application/x-ndjson")
			fmt.Fprint(w, "{\"file_id\":\"file_reference\",\"event\":\"file.processing.completed\"}\n")
		case "/backend-api/f/conversation":
			chatGPTAppImageStream(w, "data:image/gif;base64,"+gif)
		default:
			t.Errorf("unexpected path reached: %s", r.URL.Path)
			http.Error(w, `{"detail":"GIF output is not enabled for this request."}`, http.StatusBadRequest)
		}
	})
	body := fmt.Appendf(nil, `{"model":"gpt-image-2.6-sunburst","prompt":"让参考图中的主体轻轻上下浮动，保持外观和背景一致，生成首尾衔接的循环动画。","images":[{"image_url":"data:image/png;base64,%s"}],"n":1,"size":"auto","quality":"low","output_format":"gif","complexity":"high","moderation":"low","frame_count":9,"frame_duration_ms":200}`, png)
	c, rec := newOpenAIImagesTestContext(t, body)
	c.Request.URL.Path = openAIImagesEditsEndpoint
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.False(t, openAIImagesRequiresNativeProtocol(parsed))
	result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Contains(t, rec.Body.String(), `"b64_json":"`)
	var conversation []byte
	for _, call := range upstream.imageCalls() {
		if call.path == "/backend-api/f/conversation" {
			conversation = call.body
		}
	}
	require.NotEmpty(t, conversation)
	require.Contains(t, gjson.GetBytes(conversation, "messages.0.content.parts.1").String(), "low output quality")
	require.Contains(t, gjson.GetBytes(conversation, "messages.0.content.parts.1").String(), "low moderation setting")
	require.NotContains(t, string(conversation), "output_format")
	require.NotContains(t, string(conversation), "frame_count")
	require.NotContains(t, rec.Body.String(), "GIF output is not enabled")
}
