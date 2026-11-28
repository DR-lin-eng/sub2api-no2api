package service

import (
	"context"
	"net/http"
	"strings"
)

func imageSafetyRefusalCode(err *OpenAIImagesUpstreamError) string {
	if err == nil || err.SynthesizedFromModelText ||
		(err.StatusCode != http.StatusBadRequest && err.StatusCode != http.StatusForbidden && err.StatusCode != http.StatusUnprocessableEntity) {
		return ""
	}
	switch code := strings.ToLower(strings.TrimSpace(err.Code)); code {
	case "content_policy_violation", "content_filter", "content_filter_error", "moderation_blocked",
		"safety_violation", "safety_policy_violation", "responsibleaipolicyviolation":
		return code
	default:
		return ""
	}
}

// RecordImageSafetyEvent records structured upstream refusals on the existing
// bounded audit queue. Only request metadata and the recognized code are kept.
func (s *ContentModerationService) RecordImageSafetyEvent(ctx context.Context, input ContentModerationCheckInput, upstreamErr *OpenAIImagesUpstreamError) {
	if s == nil || s.repo == nil {
		return
	}
	code := imageSafetyRefusalCode(upstreamErr)
	if code == "" {
		return
	}
	snapshot, err := s.loadRuntimeSnapshot(ctx)
	if err != nil || snapshot == nil || !snapshot.riskControlEnabled {
		return
	}
	cfg := snapshot.config
	if cfg == nil || !cfg.includesGroup(input.GroupID) || !cfg.includesModel(input.Model) {
		return
	}
	// Keep this audit-only record out of violation counts on older nodes.
	log := s.buildLog(input, cfg, ContentModerationActionImageSafety, false,
		code, 0, nil, "", nil, nil, "")
	log.Mode = "post_upstream"
	s.enqueueRecord(input, cfg, log, "", false, false)
}
