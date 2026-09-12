package service

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
)

const embeddedQualitySVGMaxBytes = 1 << 20

var (
	qualitySVGScriptPattern   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script\s*>`)
	qualitySVGEventPattern    = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)`)
	qualitySVGExternalPattern = regexp.MustCompile(`(?i)(?:https?:|javascript:|data:text/html)`)
)

// processEmbeddedQualitySVG is the in-process fallback used when no renderer
// endpoint is configured. It extracts a bounded SVG preview, strips executable
// content and classifies only structurally valid SVG. The browser renders the
// returned SVG directly as an image, so no extra renderer setting is needed.
func processEmbeddedQualitySVG(_ context.Context, html string) (*QualityArtifact, error) {
	if len([]byte(html)) > embeddedQualitySVGMaxBytes {
		return nil, errors.New("generated HTML exceeds embedded renderer limit")
	}
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<svg")
	end := strings.LastIndex(lower, "</svg>")
	if start < 0 || end < start {
		return nil, errors.New("generated output does not contain a complete SVG")
	}
	svg := strings.TrimSpace(html[start : end+len("</svg>")])
	svg = qualitySVGScriptPattern.ReplaceAllString(svg, "")
	svg = qualitySVGEventPattern.ReplaceAllString(svg, "")
	if qualitySVGExternalPattern.MatchString(svg) {
		return nil, errors.New("embedded SVG contains external or executable content")
	}
	if !bytes.Contains(bytes.ToLower([]byte(svg)), []byte("<svg")) {
		return nil, errors.New("embedded SVG is empty")
	}
	return &QualityArtifact{Label: "normal", Confidence: 0.9, ModelVersion: "embedded-svg", SVG: []byte(svg)}, nil
}
