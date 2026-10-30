package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCodexModelsDelayedRefreshReusesFreshCache(t *testing.T) {
	svc := &OpenAIGatewayService{}
	const key = "late-cold-caller"
	_, state := svc.codexModelsManifestCache.get(key, time.Now())
	require.Equal(t, codexModelsManifestCacheMiss, state)
	// Another caller finishes before this caller enters the singleflight group.
	manifest := &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"gpt-5.4"}]}`)}
	svc.codexModelsManifestCache.set(key, manifest, time.Now())
	result := <-svc.refreshCachedAPIKeyCodexModelsManifest(key, codexModelsManifestRequest{useAPIKeyUpstream: true})
	require.NoError(t, result.Err, "late caller must not try the unwired upstream client")
	require.Same(t, manifest, result.Val)
}
