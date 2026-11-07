package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// Read the optional safety fields independently of the selected account. Native
// OAuth requests are rebuilt from these fields; API-key requests retain the body.
func parseAndValidateOpenAIImages26Options(req *OpenAIImagesRequest) error {
	if !req.Multipart {
		for _, field := range []struct {
			name string
			dest *string
		}{{"safety_identifier", &req.SafetyIdentifier}, {"user", &req.User}} {
			value := gjson.GetBytes(req.Body, field.name)
			if value.Exists() && value.Type != gjson.Null {
				if value.Type != gjson.String {
					return fmt.Errorf("%s must be a string", field.name)
				}
				*field.dest = value.String()
			}
		}
	}
	if utf8.RuneCountInString(req.SafetyIdentifier) > 128 {
		return fmt.Errorf("safety_identifier must be at most 128 characters")
	}
	return validateOpenAIImages26Options(req, req.Model)
}

func validateOpenAIImages26Options(req *OpenAIImagesRequest, model string) error {
	if req == nil || !usesCodexDirectImages(model) {
		return nil
	}
	if !req.Multipart {
		for _, field := range []string{"complexity", "quality", "output_format"} {
			value := gjson.GetBytes(req.Body, field)
			if value.Exists() && value.Type != gjson.Null && value.Type != gjson.String {
				return fmt.Errorf("%s must be a string", field)
			}
		}
	}
	switch req.Complexity {
	case "", "low", "medium", "high":
	default:
		return fmt.Errorf("complexity must be one of low, medium or high")
	}
	switch req.Quality {
	case "", "auto", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("quality must be one of auto, low, medium, high, xhigh or max")
	}
	if req.N > 10 {
		return fmt.Errorf("n must be between 1 and 10")
	}
	switch req.OutputFormat {
	case "", "png", "jpeg", "webp", "gif", "svg":
	default:
		return fmt.Errorf("output_format must be one of png, jpeg, webp, gif or svg")
	}
	if req.FrameCount != nil && (*req.FrameCount < 2 || *req.FrameCount > 16) {
		return fmt.Errorf("frame_count must be between 2 and 16")
	}
	if req.FrameDurationMS != nil && *req.FrameDurationMS < 20 {
		return fmt.Errorf("frame_duration_ms must be at least 20")
	}
	if (req.FrameCount != nil || req.FrameDurationMS != nil) && req.OutputFormat != "gif" {
		return fmt.Errorf("frame_count and frame_duration_ms require output_format=gif")
	}
	if req.SVG != nil {
		if req.OutputFormat != "svg" || strings.TrimSpace(req.SVG.Model) == "" {
			return fmt.Errorf("svg requires output_format=svg and a nonempty svg.model")
		}
	}
	if req.OutputFormat == "gif" || req.OutputFormat == "svg" {
		if req.Stream || req.PartialImages != nil || req.OutputCompression != nil {
			return fmt.Errorf("%s output does not support streaming, partial_images or output_compression", req.OutputFormat)
		}
	}
	if req.OutputFormat == "svg" && (req.IsEdits() || req.N != 1) {
		return fmt.Errorf("svg output requires the generations endpoint and n=1")
	}
	if req.OutputFormat == "gif" && req.HasMask {
		return fmt.Errorf("gif output does not support masks")
	}
	if !req.IsEdits() && (req.MarkupUpload != nil || gjson.GetBytes(req.Body, "markup").Type == gjson.JSON) {
		return fmt.Errorf("markup requires the edits endpoint")
	}
	return nil
}
