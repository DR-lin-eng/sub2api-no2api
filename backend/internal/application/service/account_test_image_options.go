package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

type AccountImageTestOptions struct {
	OutputFormat    string                  `json:"output_format,omitempty"`
	FrameCount      *int                    `json:"frame_count,omitempty"`
	FrameDurationMS *int                    `json:"frame_duration_ms,omitempty"`
	SVG             *OpenAIImagesSVGOptions `json:"svg,omitempty"`
}

type accountImageTestOptionsKey struct{}

func WithAccountImageTestOptions(ctx context.Context, options AccountImageTestOptions) context.Context {
	return context.WithValue(ctx, accountImageTestOptionsKey{}, options)
}

func accountImageTestOptions(ctx context.Context) AccountImageTestOptions {
	options, _ := ctx.Value(accountImageTestOptionsKey{}).(AccountImageTestOptions)
	return options
}

func buildAccountImageTestRequest(ctx context.Context, account *Account, model, prompt string) (*OpenAIImagesRequest, error) {
	options := accountImageTestOptions(ctx)
	format := strings.ToLower(strings.TrimSpace(options.OutputFormat))
	if !usesCodexDirectImages(model) && (format == "svg" || format == "gif" || options.FrameCount != nil || options.FrameDurationMS != nil || options.SVG != nil) {
		return nil, fmt.Errorf("SVG and GIF tests require a GPT Image 2.6 model")
	}
	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint, Model: strings.TrimSpace(model), Prompt: prompt,
		Size: account.OpenAIImageTestSize(), OutputFormat: format,
		FrameCount: options.FrameCount, FrameDurationMS: options.FrameDurationMS, SVG: options.SVG,
	}
	if format == "gif" && parsed.Size == "" {
		parsed.Size = "auto"
	}
	parsed.ExplicitSize = parsed.Size != ""
	applyOpenAIImagesDefaults(parsed)
	if err := validateOpenAIImages26Options(parsed, parsed.Model); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (s *AccountTestService) sendAccountImageTestResult(c *gin.Context, parsed *OpenAIImagesRequest, image, format, revised string) error {
	if revised != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: revised})
	}
	if format == "" {
		format = parsed.OutputFormat
	}
	if parsed.OutputFormat == "gif" || parsed.OutputFormat == "svg" {
		format = parsed.OutputFormat
	}
	mimeType := openAIImageOutputMIMEType(format)
	if err := validateAccountImageTestOutput(image, parsed.OutputFormat); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	s.sendEvent(c, TestEvent{Type: "image", ImageURL: "data:" + mimeType + ";base64," + image, MimeType: mimeType})
	return nil
}
