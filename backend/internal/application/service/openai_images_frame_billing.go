package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const defaultOpenAIImagesGIFFrameCount = 9

// GIFs are completed assets, with each frame billed as an image only when the
// group explicitly enables actual-image-count billing. Never multiply token usage.
func openAIImagesBillableOutputs(body []byte, parsed *OpenAIImagesRequest, count int, sizes []string, countBilling bool) (int, []string) {
	if !countBilling || parsed == nil || parsed.OutputFormat != "gif" || count <= 0 {
		return count, sizes
	}
	frames := defaultOpenAIImagesGIFFrameCount
	if parsed.FrameCount != nil && *parsed.FrameCount >= 2 && *parsed.FrameCount <= 16 {
		frames = *parsed.FrameCount
	}
	root := gjson.ParseBytes(body)
	frameCount := func(value gjson.Result, fallback int) int {
		if n, ok := boundedJSONNonNegativeInt(value); ok && n >= 2 && n <= 16 {
			return n
		}
		return fallback
	}
	frames = frameCount(root.Get("frame_count"), frames)
	var perOutput []int
	for _, item := range root.Get("data").Array() {
		if strings.TrimSpace(item.Get("b64_json").String()) == "" && strings.TrimSpace(item.Get("url").String()) == "" {
			continue
		}
		actual := frameCount(item.Get("frame_count"), frames)
		encoded := item.Get("b64_json").String()
		if encoded == "" {
			encoded, _ = strings.CutPrefix(item.Get("url").String(), "data:image/gif;base64,")
		}
		if encoded != "" {
			if decoded, err := openAIImagesGIFFrames(encoded); err == nil {
				actual = decoded
			}
		}
		perOutput = append(perOutput, actual)
	}
	total := 0
	var expanded []string
	for i := 0; i < count; i++ {
		units := frames
		if i < len(perOutput) {
			units = perOutput[i]
		}
		total += units
		if len(sizes) > 0 {
			size := ""
			if i < len(sizes) {
				size = sizes[i]
			}
			for j := 0; j < units; j++ {
				expanded = append(expanded, size)
			}
		}
	}
	return total, expanded
}
