package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/stretchr/testify/require"
)

func catalogHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestUpstreamCatalogPaginationUsesSameTransportAndIsBounded(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "base_url": "https://upstream.example/v1"}, Concurrency: 2}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{catalogHTTPResponse(`{"data":[{"id":"model-1","context_window":777}],"has_more":true,"last_id":"model-1"}`), catalogHTTPResponse(`{"data":[{"id":"model-2","max_output_tokens":123}],"has_more":false}`)}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	catalog, err := svc.FetchUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"model-1", "model-2"}, catalog.Models)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "model-1", upstream.requests[1].URL.Query().Get("after_id"))
	require.Equal(t, "777", string(catalog.Metadata["model-1"]["context_window"]))
	upstream.responses = []*http.Response{catalogHTTPResponse(`{"data":[{"id":"model-1"}],"has_more":true}`)}
	_, err = svc.FetchUpstreamModelCatalog(context.Background(), account)
	require.Error(t, err)
}

type catalogSyncRepoStub struct{ *oauthModelSyncRepoStub }

func (r *catalogSyncRepoStub) UpdateUpstreamModelCatalog(ctx context.Context, account *Account, snapshot *modelcatalog.Snapshot) error {
	return r.UpdateExtra(ctx, account.ID, map[string]any{modelcatalog.ExtraKey: snapshot})
}
func TestOAuthModelSyncCatalogFailurePreservesPreviousSnapshot(t *testing.T) {
	repo := &catalogSyncRepoStub{&oauthModelSyncRepoStub{}}
	fetcher := &openAIModelsManifestFetcherStub{manifest: &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":777777,"input_modalities":[]}]}`)}}
	service := NewOAuthModelSyncService(repo, &AccountTestService{openAIModelsManifest: fetcher}, OAuthModelSyncOptions{})
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, service.syncAccount(context.Background(), account))
	before := repo.updates[account.ID][modelcatalog.ExtraKey]
	require.NotNil(t, before)
	fetcher.manifest.Body = []byte(`{"models":[{"slug":"bad model"}]}`)
	require.False(t, service.syncAccount(context.Background(), account))
	require.Same(t, before, repo.updates[account.ID][modelcatalog.ExtraKey])
	fetcher.manifest.Body = []byte(`{"models":[{"slug":"new-model"}]}`)
	require.True(t, service.syncAccount(context.Background(), account))
	snapshot, ok := repo.updates[account.ID][modelcatalog.ExtraKey].(*modelcatalog.Snapshot)
	require.True(t, ok)
	require.Contains(t, snapshot.Models, "new-model")
	require.NotContains(t, snapshot.Models, "gpt-6.1-sol")
}
