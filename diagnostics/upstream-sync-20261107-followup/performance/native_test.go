//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var followupBytesSink []byte
var followupStringSink string

// Exact whole-body set/delete strategy in the frozen upstream native response path.
func followupUpstreamImageRewrite(body []byte, parsed *OpenAIImagesRequest) []byte {
	for i := range gjson.GetBytes(body, "data").Array() {
		body, _ = sjson.SetBytes(body, fmt.Sprintf("data.%d.model", i), parsed.Model)
	}
	for i, item := range gjson.GetBytes(body, "data").Array() {
		if actual := detectOpenAIImageResultSize(item.Get("b64_json").String()); actual != "" {
			body, _ = sjson.SetBytes(body, fmt.Sprintf("data.%d.size", i), actual)
			if i == 0 {
				body, _ = sjson.SetBytes(body, "size", actual)
			}
		}
	}
	for i := range gjson.GetBytes(body, "data").Array() {
		body = followupCodexDirectImageURL(body, fmt.Sprintf("data.%d.", i), "png")
	}
	return body
}

func BenchmarkSyncNativeImageRewrite(b *testing.B) {
	items := make([]map[string]string, 8)
	for i := range items {
		items[i] = map[string]string{"b64_json": strings.Repeat("A", 256<<10)}
	}
	body, _ := json.Marshal(map[string]any{"data": items, "future": true})
	parsed := &OpenAIImagesRequest{Model: "gpt-image-2.6-flare", ResponseFormat: "url"}
	for _, mode := range []string{"upstream", "linear"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if mode == "upstream" {
					followupBytesSink = followupUpstreamImageRewrite(body, parsed)
				} else {
					var err error
					followupBytesSink, err = rewriteCodexDirectImagesResponse(body, parsed)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkSyncTerraInstructions(b *testing.B) {
	for _, mode := range []string{"per-request-decode", "startup-cache"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if mode == "per-request-decode" {
					var descriptor struct {
						ModelMessages struct {
							Instructions string `json:"instructions_template"`
						} `json:"model_messages"`
					}
					if err := json.Unmarshal(openai.CodexGPT61TerraMetadata, &descriptor); err != nil {
						b.Fatal(err)
					}
					followupStringSink = descriptor.ModelMessages.Instructions
				} else {
					followupStringSink = openai.CodexBaseInstructionsForModel("gpt-6.1-terra")
				}
			}
		})
	}
}

func followupCodexDirectImageURL(body []byte, path, format string) []byte {
	if result := gjson.GetBytes(body, path+"b64_json").String(); result != "" {
		body, _ = sjson.SetBytes(body, path+"url", "data:"+openAIImageOutputMIMEType(format)+";base64,"+result)
		body, _ = sjson.DeleteBytes(body, path+"b64_json")
	}
	return body
}
