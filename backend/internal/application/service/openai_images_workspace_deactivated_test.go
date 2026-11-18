//go:build unit

package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

const workspaceDeactivatedTestBody = `{"error":{"code":"deactivated_workspace","message":"Payment Required","type":"upstream_error"}}`

func TestChatGPTAppImagesWorkspaceDeactivatedFailsOver(t *testing.T) {
	for _, stage := range []string{"prepare", "upload", "generation"} {
		for _, stream := range []bool{false, true} {
			for _, cleanup := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/cleanup=%t", stage, stream, cleanup), func(t *testing.T) {
					reject := func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusPaymentRequired)
						fmt.Fprint(w, workspaceDeactivatedTestBody)
					}
					var prepare http.HandlerFunc
					if stage == "prepare" {
						prepare = reject
					}
					svc, upstream := newChatGPTAppImageTestServiceWithPrepare(t, prepare, reject)
					repo := &rateLimitAccountRepoStub{deleteApplied: true, deleteIDs: []int64{811}}
					svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
					svc.rateLimitService.SetSettingService(oauth401AutoDeleteSettingService(t, cleanup))
					svc.rateLimitService.SetAccountRuntimeBlocker(svc)
					body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-sunburst","prompt":"draw","stream":%t}`, stream))
					if stage == "upload" {
						body = []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-sunburst","prompt":"edit","stream":%t,"images":[{"image_url":"data:image/png;base64,%s"}]}`, stream, encodeOpenAIImageTestPNG(t, 2, 2)))
					}
					c := newOpenAIImagesContext(t, body)
					if stage == "upload" {
						c.Request.URL.Path = "/v1/images/edits"
					}
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					account := chatGPTAppImageTestAccount()
					account.Status, account.Schedulable = StatusActive, true

					result, err := svc.ForwardImages(t.Context(), c, account, body, parsed, "")

					require.Nil(t, result)
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, http.StatusPaymentRequired, failover.StatusCode)
					require.JSONEq(t, workspaceDeactivatedTestBody, string(failover.ResponseBody))
					require.Equal(t, -1, OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), "only heartbeat frames may be written before account failover")
					require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
					if cleanup {
						require.Equal(t, 1, repo.deleteCalls)
						require.Zero(t, repo.setErrorCalls)
					} else {
						require.Zero(t, repo.deleteCalls)
						require.Equal(t, 1, repo.setErrorCalls)
					}
					if stage == "prepare" {
						require.Empty(t, upstream.imageCalls())
					} else {
						require.Len(t, upstream.imageCalls(), 1, "the failing attempt must not start a second generation")
					}
				})
			}
		}
	}
}

func TestChatGPTAppImagesWorkspaceDeactivatedAfterGenerationRetainsSettlement(t *testing.T) {
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/f/conversation" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"tool\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"content_type\":\"image_asset_pointer\",\"asset_pointer\":\"sediment://file_output\",\"width\":3,\"height\":3}]}}}\n\ndata: [DONE]\n\n")
			return
		}
		require.Equal(t, "/backend-api/files/download/file_output", r.URL.Path)
		w.WriteHeader(http.StatusPaymentRequired)
		fmt.Fprint(w, workspaceDeactivatedTestBody)
	})
	repo := &rateLimitAccountRepoStub{}
	svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")

	var terminal *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &terminal)
	require.Equal(t, http.StatusPaymentRequired, terminal.StatusCode)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Len(t, upstream.imageCalls(), 2, "an accepted generation must never be replayed after a download failure")
}
