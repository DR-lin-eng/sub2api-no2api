package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/modules/qualityrender"
)

// AccountQualityCodeMatch records the policy used for this particular answer.
// Historical records retain the old verdict even after an administrator edits it.
type AccountQualityCodeMatch struct {
	qualityrender.CodeMatch
	NormalClass string `json:"normal_class"`
}

func (s *AccountQualityMonitoringService) matchQualityDrawing(ctx context.Context, source string, settings AccountQualitySettings, outcome *qualityStageOutcome, beforeRender []func()) {
	match, err := qualityrender.MatchHTML(source, settings.CodeMatchThreshold)
	if err != nil {
		outcome.errorMessage = err.Error()
		return
	}
	outcome.detail.CodeMatch = &AccountQualityCodeMatch{CodeMatch: *match, NormalClass: settings.CodeMatchNormalClass}
	passed := match.IsModelA == (settings.CodeMatchNormalClass != "other")
	outcome.operational, outcome.passed = false, passed
	outcome.status = "wrong"
	label := "unnormal"
	if passed {
		outcome.status, label = "passed", "normal"
	}
	// The legacy confidence field remains zero: a source-code similarity score
	// is not a classifier probability. Only PNG/WebP are accepted from renderers.
	outcome.artifact = &QualityArtifact{Label: label, ModelVersion: match.Version}
	outcome.detail.PreviewStatus = "error"
	if s.qualityProcessor == nil {
		outcome.errorMessage = "preview renderer is unavailable"
		return
	}
	for _, callback := range beforeRender {
		if callback != nil {
			callback()
		}
	}
	preview, err := s.qualityProcessor.Process(ctx, source)
	if err != nil || preview == nil || len(preview.PNG) == 0 || len(preview.WebP) == 0 {
		outcome.errorMessage = "preview rendering failed; code match retained"
		if ctx.Err() != nil {
			outcome.status, outcome.passed, outcome.operational = "error", false, true
			outcome.errorMessage = ctx.Err().Error()
		}
		return
	}
	outcome.artifact.PNG, outcome.artifact.WebP = preview.PNG, preview.WebP
	outcome.detail.PreviewStatus = "ready"
}
