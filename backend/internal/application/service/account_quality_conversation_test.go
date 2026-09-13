package service

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type qualityConversationStore struct {
	AccountQualityArtifactRepository
	runs []AccountQualityRun
}

func (s *qualityConversationStore) Save(_ context.Context, run *AccountQualityRun) error {
	run.ID = "719a94e5-4f93-4ab8-a495-4b430be08a10"
	run.HasPreview = len(run.WebP) > 0
	s.runs = append(s.runs, *run)
	return nil
}
func (s *qualityConversationStore) ListPublic(context.Context, time.Time, int) ([]AccountQualityRun, error) {
	return s.runs, nil
}

func TestQualityConversationCapturesProviderIdentityWithoutReasoningText(t *testing.T) {
	cases := []struct {
		name, stream, conversation, response string
		read                                 func(*AccountTestService, *gin.Context, io.Reader) error
	}{
		{"responses", `data: {"type":"response.created","response":{"id":"resp_1","conversation":{"id":"conv_1"}}}` + "\n\n" + `data: {"type":"response.reasoning_text.delta","delta":"private thought"}` + "\n\n" + `data: {"type":"response.output_text.delta","delta":"21"}` + "\n\n" + `data: {"type":"response.completed","response":{"id":"resp_1"}}` + "\n\n", "conv_1", "resp_1", (*AccountTestService).processOpenAIStream},
		{"chat", `data: {"id":"chatcmpl-1","choices":[{"delta":{"reasoning_content":"private thought","content":"21"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", "", "chatcmpl-1", (*AccountTestService).processOpenAIChatCompletionsStream},
		{"gemini", `data: {"response":{"responseId":"gemini-1","candidates":[{"content":{"parts":[{"thought":true,"text":"private thought"},{"text":"21"}]},"finishReason":"STOP"}]}}` + "\n\n", "", "gemini-1", (*AccountTestService).processGeminiStream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = (&http.Request{}).WithContext(withAccountTestUsage(context.Background()))
			require.NoError(t, tc.read(&AccountTestService{}, c, strings.NewReader(tc.stream)))
			conversation, response := parseQualityResponseIdentity(w.Body.String())
			require.Equal(t, tc.conversation, conversation)
			require.Equal(t, tc.response, response)
			answer, _ := parseTestSSEOutput(w.Body.String())
			require.Equal(t, "21", answer)
			require.NotContains(t, w.Body.String(), "private thought")
		})
	}
}

func TestQualityConversationPersistsTextOnlyAndCombinedVerdict(t *testing.T) {
	for _, tc := range []struct {
		name, answer, status string
		drawing              bool
	}{{"text only", "21", "ready", false}, {"wrong text normal image", "20", "wrong", true}, {"both passed", "21", "ready", true}} {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultAccountQualitySettings()
			settings.Enabled = true
			settings.PublicEnabled = true
			settings.Stage2Enabled = tc.drawing
			config, _ := json.Marshal(settings)
			store := &qualityConversationStore{}
			probe := &qualityStageProbeStub{results: []*ScheduledTestResult{{Status: "success", ResponseText: tc.answer, ConversationID: "conv_text", ResponseID: "resp_text", ReasoningTokens: reasoningTokenPtr(0)}, {Status: "success", ResponseText: modelAHTML, ResponseID: "resp_image"}}}
			svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: probe, qualityProcessor: &qualityStageProcessorStub{}, qualityArtifacts: store, settingRepo: &inspectionSettingRepoStub{values: map[string]string{SettingKeyAccountQualitySettings: string(config)}}}
			account := Account{ID: 123456, Name: "private account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
			rows := []AccountInspectionAccountResult{{AccountID: account.ID}}
			require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, rows, nil, settings, time.Now()))
			require.Len(t, store.runs, 1)
			run := store.runs[0]
			require.Equal(t, tc.status, run.Status)
			require.Equal(t, tc.answer, run.Details.Stage1.Answer)
			require.Equal(t, "conv_text", run.Details.Stage1.ConversationID)
			require.Equal(t, reasoningTokenPtr(0), run.Details.Stage1.ReasoningTokens)
			snapshot, err := svc.GetPublicQualitySnapshot(context.Background())
			require.NoError(t, err)
			require.Len(t, snapshot.Points, 1)
			if tc.status == "wrong" {
				require.Equal(t, 1, snapshot.Degraded)
			} else {
				require.Equal(t, 1, snapshot.Passed)
			}
			encoded, _ := json.Marshal(snapshot)
			require.NotContains(t, string(encoded), "123456")
			require.NotContains(t, string(encoded), "private account")
			require.NotContains(t, string(encoded), "error_message")
		})
	}
}

func TestQualityConversationBoundsUTF8AndValidatesIDs(t *testing.T) {
	detail := qualityStageDetail(&ScheduledTestResult{ResponseText: strings.Repeat("汉", 6000), ConversationID: "https://upstream.test/?token=secret", ResponseID: strings.Repeat("a", 257)})
	require.True(t, detail.AnswerTruncated)
	require.LessOrEqual(t, len(detail.Answer), 16384)
	require.True(t, utf8.ValidString(detail.Answer))
	require.Empty(t, detail.ConversationID)
	require.Empty(t, detail.ResponseID)
	detail = qualityStageDetail(&ScheduledTestResult{ResponseText: "a\x00b"})
	require.Equal(t, "ab", detail.Answer)
}

func TestQualityConversationPublicDisabledAndLegacy(t *testing.T) {
	svc := &AccountQualityMonitoringService{settingRepo: &inspectionSettingRepoStub{values: map[string]string{}}}
	_, err := svc.GetPublicQualitySnapshot(context.Background())
	require.Error(t, err)
	settings := DefaultAccountQualitySettings()
	settings.Enabled = true
	settings.PublicEnabled = true
	config, _ := json.Marshal(settings)
	svc.settingRepo = &inspectionSettingRepoStub{values: map[string]string{SettingKeyAccountQualitySettings: string(config)}}
	svc.qualityArtifacts = &qualityConversationStore{runs: []AccountQualityRun{{ID: "old-record", Status: "ready", Label: "normal", HasPreview: true}}}
	snapshot, err := svc.GetPublicQualitySnapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.Passed)
	require.Nil(t, snapshot.Points[0].Details.Stage1)
	require.True(t, snapshot.Points[0].HasPreview)
}
