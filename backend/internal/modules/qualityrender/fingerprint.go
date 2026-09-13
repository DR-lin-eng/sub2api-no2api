package qualityrender

import (
	"errors"
	"math"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const FingerprintVersion = "code-fingerprint-v1"
const DefaultMatchThreshold = 55.0
const MaxHTMLBytes = 1 << 20

// CodeMatch is a weighted source-code similarity score, not a probability or
// proof of model identity. Signal names, weights and case sensitivity mirror
// the supplied model_a_fingerprint.py reference.
type CodeMatch struct {
	Version        string   `json:"version"`
	Score          float64  `json:"score"`
	RawPoints      int      `json:"raw_points"`
	MaxPoints      int      `json:"max_points"`
	MatchedSignals []string `json:"matched_signals"`
	MissingSignals []string `json:"missing_signals"`
	IsModelA       bool     `json:"is_model_a"`
	Threshold      float64  `json:"threshold"`
}

var (
	trigPattern = regexp.MustCompile(`Math\.(cos|sin)`)
	// Python's Unicode \s includes the four information separators as well.
	usePattern      = regexp.MustCompile(`(?i)<use[[:space:]\p{Z}\x{85}\x{1c}-\x{1f}]`)
	buttonPattern   = regexp.MustCompile(`(?i)<button`)
	commentPattern  = regexp.MustCompile(`(?s)<!--.*?-->`)
	layerPattern    = regexp.MustCompile(`class="(sun|cloud|hill)"`)
	titlePattern    = regexp.MustCompile(`(?i)<title\s[^>]*id="([\w-]+)"`)
	descPattern     = regexp.MustCompile(`(?i)<desc\s[^>]*id="([\w-]+)"`)
	labelledPattern = regexp.MustCompile(`(?i)aria-labelledby="([^"]+)"`)
)

func MatchHTML(source string, threshold float64) (*CodeMatch, error) {
	if len(source) > MaxHTMLBytes || !containsSVG(source) {
		return nil, errors.New("code matching requires HTML/SVG within 1 MiB")
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		return nil, errors.New("code match threshold must be between 0 and 100")
	}
	naming := false
	for _, name := range []string{"scarf-tail", "far-leg", "near-leg", "far-foot", "near-foot", "far-pedal", "near-pedal", "wheel-turn", "crank-turn", "cloud-strip", "hill-strip", "road-strip", "flower-strip"} {
		if strings.Contains(source, name) {
			naming = true
			break
		}
	}
	signals := []struct {
		name   string
		weight int
		hit    bool
	}{
		{"js_trig_leg_animation", 20, trigPattern.MatchString(source) && strings.Contains(source, "requestAnimationFrame")},
		{"defs_use_reuse", 15, len(usePattern.FindAllStringIndex(source, 6)) >= 6},
		{"reduced_motion_js", 12, strings.Contains(source, "prefers-reduced-motion") && strings.Contains(source, "matchMedia")},
		{"play_pause_button", 12, strings.Contains(source, "aria-pressed") && buttonPattern.MatchString(source)},
		{"title_desc_aria_labelledby", 10, svgTitleDescLabelledBy(source)},
		{"visibilitychange_listener", 8, strings.Contains(source, "visibilitychange")},
		{"multiword_kebab_naming", 8, naming},
		{"no_root_css_vars", 7, !strings.Contains(commentPattern.ReplaceAllString(source, ""), ":root")},
		{"pure_svg_scene", 8, !layerPattern.MatchString(source)},
	}
	result := &CodeMatch{Version: FingerprintVersion, Threshold: threshold, MatchedSignals: []string{}, MissingSignals: []string{}}
	for _, signal := range signals {
		result.MaxPoints += signal.weight
		if signal.hit {
			result.RawPoints += signal.weight
			result.MatchedSignals = append(result.MatchedSignals, signal.name)
		} else {
			result.MissingSignals = append(result.MissingSignals, signal.name)
		}
	}
	result.Score = math.Round(1000*float64(result.RawPoints)/float64(result.MaxPoints)) / 10
	result.IsModelA = result.Score >= threshold
	return result, nil
}

func svgTitleDescLabelledBy(source string) bool {
	titles, descriptions := map[string]struct{}{}, map[string]struct{}{}
	for _, match := range titlePattern.FindAllStringSubmatch(source, -1) {
		titles[match[1]] = struct{}{}
	}
	for _, match := range descPattern.FindAllStringSubmatch(source, -1) {
		descriptions[match[1]] = struct{}{}
	}
	if len(titles) == 0 || len(descriptions) == 0 {
		return false
	}
	for _, match := range labelledPattern.FindAllStringSubmatch(source, -1) {
		var hasTitle, hasDescription bool
		for _, id := range strings.Fields(match[1]) {
			if _, ok := titles[id]; ok {
				hasTitle = true
			}
			if _, ok := descriptions[id]; ok {
				hasDescription = true
			}
		}
		if hasTitle && hasDescription {
			return true
		}
	}
	return false
}

// An empty answer, escaped markup, comments or a script containing an SVG
// string are not drawings and must never receive a score from negative signals.
func containsSVG(source string) bool {
	tokens := html.NewTokenizer(strings.NewReader(source))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokens.TagName()
			if string(name) == "svg" {
				return true
			}
		}
	}
}
