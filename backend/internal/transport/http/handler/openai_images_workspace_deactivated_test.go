//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	middleware2 "github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type workspaceDeactivatedImagesRepo struct {
	openAIImagesFailoverAccountRepo
	disabled []int64
}

func (r *workspaceDeactivatedImagesRepo) SetError(_ context.Context, id int64, message string) error {
	r.disabled = append(r.disabled, id)
	return nil
}

type workspaceDeactivatedImagesUpstream struct {
	openAIImagesFailoverHTTPUpstream
}

func (u *workspaceDeactivatedImagesUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/sentinel/chat-requirements/prepare") {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"token":"fixture-requirements-token"}`))}, nil
	}
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusPaymentRequired,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"deactivated_workspace","message":"Payment Required","type":"upstream_error"}}`)),
	}, nil
}

func TestOpenAIGatewayHandlerImages_WorkspaceDeactivatedFailsOver(t *testing.T) {
	for _, mode := range []string{"app_oauth", "legacy_oauth", "api_key"} {
		for _, endpoint := range []string{"generations", "edits"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", mode, endpoint, stream), func(t *testing.T) {
					accounts := []service.Account{
						{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 0, Credentials: map[string]any{"access_token": "token-1"}},
						{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"access_token": "token-2"}},
					}
					if mode == "api_key" {
						for i := range accounts {
							accounts[i].Type = service.AccountTypeAPIKey
							accounts[i].Credentials = map[string]any{"api_key": "sk-test"}
						}
					}
					repo := &workspaceDeactivatedImagesRepo{openAIImagesFailoverAccountRepo: openAIImagesFailoverAccountRepo{accounts: accounts}}
					cfg := &config.Config{RunMode: config.RunModeSimple}
					limits := service.NewRateLimitService(repo, nil, cfg, nil, nil)
					upstream := &workspaceDeactivatedImagesUpstream{}
					gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, limits, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
					billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
					t.Cleanup(billing.Stop)
					h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
					body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-sunburst","prompt":"draw","stream":%t}`, stream))
					if endpoint == "edits" {
						var input bytes.Buffer
						require.NoError(t, png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 1, 1))))
						body = []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-sunburst","prompt":"edit","stream":%t,"images":[{"image_url":"data:image/png;base64,%s"}]}`, stream, base64.StdEncoding.EncodeToString(input.Bytes())))
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/"+endpoint, bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Set("openai_images_legacy_snapshot", mode == "legacy_oauth")
					groupID := int64(3131)
					c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 99, GroupID: &groupID, Group: &service.Group{ID: groupID, AllowImageGeneration: true}, User: &service.User{ID: 100}})
					c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100})

					h.Images(c)

					require.Equal(t, []int64{1, 2}, upstream.calls(), "each failed account must be excluded before selecting another")
					require.Equal(t, []int64{1, 2}, repo.disabled)
					require.Contains(t, rec.Body.String(), "upstream_error")
					if stream {
						require.Equal(t, http.StatusOK, rec.Code)
						require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
					} else {
						require.Equal(t, http.StatusBadGateway, rec.Code)
					}
				})
			}
		}
	}
}
