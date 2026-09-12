package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

type AccountQualityPublicPoint struct {
	ID         string                     `json:"id"`
	Status     string                     `json:"status"`
	Label      string                     `json:"label,omitempty"`
	Confidence float64                    `json:"confidence,omitempty"`
	Model      string                     `json:"model,omitempty"`
	Effort     string                     `json:"effort,omitempty"`
	LatencyMs  int64                      `json:"latency_ms,omitempty"`
	StartedAt  time.Time                  `json:"started_at"`
	Details    AccountQualityProbeDetails `json:"details"`
	HasPreview bool                       `json:"has_preview"`
	PreviewFormat string                  `json:"preview_format,omitempty"`
}

type AccountQualityPublicSnapshot struct {
	Model           string                      `json:"model"`
	Effort          string                      `json:"effort"`
	IntervalMinutes int                         `json:"interval_minutes"`
	Now             time.Time                   `json:"now"`
	LastRunAt       *time.Time                  `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time                  `json:"next_run_at,omitempty"`
	Total           int                         `json:"total"`
	Passed          int                         `json:"passed"`
	Degraded        int                         `json:"degraded"`
	Uncertain       int                         `json:"uncertain"`
	Error           int                         `json:"error"`
	ModelVersion    string                      `json:"model_version,omitempty"`
	Points          []AccountQualityPublicPoint `json:"points"`
}

func (s *AccountQualityMonitoringService) GetPublicQualitySnapshot(ctx context.Context) (*AccountQualityPublicSnapshot, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !settings.PublicEnabled {
		return nil, infraerrors.NotFound("ACCOUNT_QUALITY_PUBLIC_DISABLED", "account quality public share is disabled")
	}
	now := time.Now().UTC()
	result := &AccountQualityPublicSnapshot{Model: settings.Model, Effort: settings.Effort, IntervalMinutes: settings.IntervalMinutes, Now: now, Points: []AccountQualityPublicPoint{}}
	state, stateErr := s.loadState(ctx)
	if stateErr == nil && state != nil {
		result.LastRunAt = state.LastRunAt
		result.NextRunAt = state.NextRunAt
		if result.NextRunAt == nil && state.LastRunAt != nil {
			next := state.LastRunAt.Add(time.Duration(settings.IntervalMinutes) * time.Minute)
			result.NextRunAt = &next
		}
	}
	if s.qualityArtifacts != nil {
		runs, listErr := s.qualityArtifacts.ListPublic(ctx, now.Add(-24*time.Hour), 200)
		if listErr != nil {
			return nil, listErr
		}
		for _, run := range runs {
			result.Total++
			switch run.Status {
			case "ready":
				if run.Details.Stage1 != nil || run.Details.Stage2 != nil {
					result.Passed++
					break
				}
				switch run.Label {
				case "normal":
					result.Passed++
				case "unnormal":
					result.Degraded++
				default:
					result.Uncertain++
				}
			case "wrong":
				result.Degraded++
			case "uncertain":
				result.Uncertain++
			default:
				result.Error++
			}
			if result.ModelVersion == "" {
				result.ModelVersion = run.ModelVersion
			}
			result.Points = append(result.Points, AccountQualityPublicPoint{ID: run.ID, Status: run.Status, Label: run.Label, Confidence: run.Confidence, Model: run.Model, Effort: run.Effort, LatencyMs: run.LatencyMs, StartedAt: run.StartedAt, Details: run.Details, HasPreview: run.HasPreview, PreviewFormat: run.PreviewFormat})
		}
		return result, nil
	}
	if state != nil {
		for _, item := range state.Results {
			if item.ObservedAt.Before(now.Add(-24*time.Hour)) || item.QualityStatus == "" || item.QualityStatus == "disabled" {
				continue
			}
			result.Total++
			switch item.QualityStatus {
			case "healthy":
				result.Passed++
			case "degraded":
				result.Degraded++
			case "error":
				result.Error++
			default:
				result.Uncertain++
			}
			result.Points = append(result.Points, AccountQualityPublicPoint{Status: item.QualityStatus, LatencyMs: item.QualityLatencyMs, StartedAt: item.ObservedAt})
		}
	}
	return result, nil
}

func (s *AccountQualityMonitoringService) GetPublicQualityImage(ctx context.Context, id, format string) ([]byte, string, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, "", err
	}
	if !settings.Enabled || !settings.PublicEnabled || s.qualityArtifacts == nil {
		return nil, "", infraerrors.NotFound("ACCOUNT_QUALITY_PUBLIC_DISABLED", "account quality public share is disabled")
	}
	return s.qualityArtifacts.GetPublicImage(ctx, id, format)
}
