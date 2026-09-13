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

func (s *AccountQualityMonitoringService) matchQualityDrawing(_ context.Context, source string, settings AccountQualitySettings, outcome *qualityStageOutcome, _ []func()) {
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
	// is not a classifier probability. Preview HTML is rendered by the frontend.
	outcome.artifact = &QualityArtifact{Label: label, ModelVersion: match.Version}
	if outcome.detail.PreviewHTML != "" {
		outcome.detail.PreviewStatus = "ready"
	} else {
		outcome.detail.PreviewStatus = "unavailable"
	}
}
