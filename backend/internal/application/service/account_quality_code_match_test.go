package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 55 points: baseline 15 + trig 20 + reduced motion 12 + visibility 8.
const modelAHTML = `<svg/><script>Math.sin(0);requestAnimationFrame(tick);matchMedia("prefers-reduced-motion");document.addEventListener("visibilitychange",()=>{});</script>`

type qualityPreviewFailure struct{}

func (qualityPreviewFailure) Process(context.Context, string) (*QualityArtifact, error) {
	return nil, errors.New("preview runtime error")
}

func TestQualityCodeMatchOverridesLegacyClassifierAndRetainsPreview(t *testing.T) {
	for _, tc := range []struct {
		name, source, normalClass, status string
		threshold                         float64
	}{
		{"at threshold", modelAHTML, "model_a", "passed", 55},
		{"above score", modelAHTML, "model_a", "wrong", 56},
		{"other style", "<svg/>", "model_a", "wrong", 55},
		{"reverse normal", "<svg/>", "other", "passed", 55},
		{"reverse wrong", modelAHTML, "other", "wrong", 55},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultAccountQualitySettings()
			settings.CodeMatchThreshold, settings.CodeMatchNormalClass = tc.threshold, tc.normalClass
			settings.MinConfidence = 1 // Never applies to code similarity.
			probe := &qualityStageProbeStub{responses: []string{tc.source}}
			preview := &qualityStageProcessorStub{}
			svc := &AccountQualityMonitoringService{accountTestSvc: probe, qualityProcessor: preview}
			result := svc.runQualityStage(context.Background(), Account{}, settings, "stage2", "drawing")
			require.Equal(t, tc.status, result.status)
			require.False(t, result.operational)
			require.Equal(t, 1, preview.calls)
			require.Equal(t, "ready", result.detail.PreviewStatus)
			require.Equal(t, []byte("preview png"), result.artifact.PNG)
			require.Equal(t, []byte("preview webp"), result.artifact.WebP)
			require.Equal(t, "code-fingerprint-v1", result.artifact.ModelVersion)
			require.Zero(t, result.artifact.Confidence)
			require.Equal(t, tc.threshold, result.detail.CodeMatch.Threshold)
		})
	}
}

func TestQualityCodeMatchUsesFullAnswerAndSurvivesPreviewFailure(t *testing.T) {
	settings := DefaultAccountQualitySettings()
	settings.Enabled, settings.PublicEnabled, settings.Stage1Enabled = true, true, false
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	source := "<svg></svg>" + strings.Repeat(" ", 17000) + modelAHTML
	store := &qualityConversationStore{}
	svc := &AccountQualityMonitoringService{
		accountRepo:      &qualityRepoStub{extra: map[int64]map[string]any{}},
		accountTestSvc:   &qualityStageProbeStub{responses: []string{source}},
		qualityProcessor: qualityPreviewFailure{}, qualityArtifacts: store,
		settingRepo: &inspectionSettingRepoStub{values: map[string]string{SettingKeyAccountQualitySettings: string(raw)}},
	}
	account := Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
	rows := []AccountInspectionAccountResult{{AccountID: 10}}
	require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, rows, nil, settings, time.Now()))
	require.Equal(t, "healthy", rows[0].QualityStatus)
	require.Equal(t, 55.0, rows[0].QualityCodeMatch.Score)
	require.Equal(t, "ready", store.runs[0].Status)
	snapshot, err := svc.GetPublicQualitySnapshot(context.Background())
	require.NoError(t, err)
	point := snapshot.Points[0]
	require.False(t, point.HasPreview)
	require.True(t, point.Details.Stage2.AnswerTruncated)
	require.Equal(t, "error", point.Details.Stage2.PreviewStatus)
	require.Equal(t, 55.0, point.Details.Stage2.CodeMatch.Score)
	// Verify the JSONB/public projection retains the complete matching evidence.
	encoded, err := json.Marshal(point.Details)
	require.NoError(t, err)
	var decoded AccountQualityProbeDetails
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, point.Details.Stage2.CodeMatch, decoded.Stage2.CodeMatch)
}

func TestQualityCodeMatchRejectsNonDrawingWithoutCallingRenderer(t *testing.T) {
	for _, source := range []string{"", "no drawing", "<!-- <svg/> -->", `<script>const s="<svg/>";</script>`, "<svg/>" + strings.Repeat("x", 1<<20)} {
		preview := &qualityStageProcessorStub{}
		svc := &AccountQualityMonitoringService{accountTestSvc: &qualityStageProbeStub{responses: []string{source}}, qualityProcessor: preview}
		result := svc.runQualityStage(context.Background(), Account{}, DefaultAccountQualitySettings(), "stage2", "drawing")
		require.Equal(t, "error", result.status)
		require.True(t, result.operational)
		require.Nil(t, result.detail.CodeMatch)
		require.Zero(t, preview.calls)
	}
}

func TestQualityCodeMatchSettingsPreserveLegacyValues(t *testing.T) {
	legacy := `{"enabled":true,"min_confidence":0.97,"stage1_enabled":false,"stage2_enabled":true,"stage2_prompt":"custom drawing","model":"custom model","timeout_seconds":180}`
	repo := &inspectionSettingRepoStub{values: map[string]string{SettingKeyAccountQualitySettings: legacy}}
	svc := &AccountQualityMonitoringService{settingRepo: repo}
	settings, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 55.0, settings.CodeMatchThreshold)
	require.Equal(t, "model_a", settings.CodeMatchNormalClass)
	require.Equal(t, 0.97, settings.MinConfidence)
	require.Equal(t, "custom drawing", settings.Stage2Prompt)
	require.Equal(t, legacy, repo.values[SettingKeyAccountQualitySettings])
	settings.CodeMatchThreshold, settings.CodeMatchNormalClass = 67, "other"
	_, err = svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	saved, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 67.0, saved.CodeMatchThreshold)
	require.Equal(t, "other", saved.CodeMatchNormalClass)
	require.Equal(t, 0.97, saved.MinConfidence)
	for _, value := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		settings.CodeMatchThreshold = value
		require.Error(t, settings.validate())
	}
	settings.CodeMatchThreshold, settings.CodeMatchNormalClass = 55, "unknown"
	require.Error(t, settings.validate())
}
