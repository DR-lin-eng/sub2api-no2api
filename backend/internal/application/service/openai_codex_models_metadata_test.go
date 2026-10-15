package service

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
	"time"
)

func catalogMetadataFixture(t *testing.T, context int) *modelcatalog.Snapshot {
	t.Helper()
	raw, _ := json.Marshal(context)
	c := &modelcatalog.Catalog{Models: []string{"gpt-6.1-sol"}, Metadata: map[string]modelcatalog.Descriptor{"gpt-6.1-sol": {"context_window": raw, "supports_reasoning": json.RawMessage(`false`), "input_modalities": json.RawMessage(`[]`)}}}
	snapshot, err := c.Snapshot("fixture", time.Now())
	require.NoError(t, err)
	return snapshot
}
func TestCodexCatalogMetadataPrecedenceIsolationAndETag(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"alias": "gpt-6.1-sol"}}, Extra: map[string]any{modelcatalog.ExtraKey: catalogMetadataFixture(t, 600000)}}
	manifest := &CodexModelsManifest{Body: []byte(`{"unknown":"preserved","models":[{"slug":"gpt-6.1-sol","max_output_tokens":32768}]}`), ETag: `"upstream"`}
	original := append([]byte(nil), manifest.Body...)
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, int64(600000), gjson.GetBytes(projected.Body, "models.0.context_window").Int())
	require.Equal(t, "alias", gjson.GetBytes(projected.Body, "models.0.slug").String())
	require.Equal(t, "[]", gjson.GetBytes(projected.Body, "models.0.input_modalities").Raw)
	require.False(t, gjson.GetBytes(projected.Body, "models.0.supports_reasoning").Bool())
	require.Equal(t, "preserved", gjson.GetBytes(projected.Body, "unknown").String())
	require.Equal(t, original, manifest.Body)
	conditional, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, projected.ETag)
	require.NoError(t, err)
	require.True(t, conditional.NotModified)
	other := *account
	other.ID = 2
	other.Extra = map[string]any{modelcatalog.ExtraKey: catalogMetadataFixture(t, 700000)}
	second, err := ProjectCodexModelsManifestForAccount(manifest, &other, &Group{}, projected.ETag)
	require.NoError(t, err)
	require.False(t, second.NotModified)
	require.NotEqual(t, projected.ETag, second.ETag)
	require.Equal(t, int64(700000), gjson.GetBytes(second.Body, "models.0.context_window").Int())
	manifest.Body = []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":777777,"supports_reasoning":false,"input_modalities":[]}]}`)
	projected, err = ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, int64(777777), gjson.GetBytes(projected.Body, "models.0.context_window").Int())
	account.Extra["model_metadata_overrides"] = map[string]any{"alias": map[string]any{"context_window": 800000}}
	projected, err = ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, int64(800000), gjson.GetBytes(projected.Body, "models.0.context_window").Int())
}
func TestCodexStandardListMetadataAndKnownFallback(t *testing.T) {
	body := convertOpenAIModelListToCodexManifest([]byte(`{"data":[{"id":"gpt-6.1-sol","context_window":777777,"max_output_tokens":32768,"supports_reasoning":false,"input_modalities":[]},{"id":"gpt-6.1-sol-2026-10-01"}]}`))
	require.Equal(t, int64(777777), gjson.GetBytes(body, "models.0.context_window").Int())
	require.Equal(t, int64(32768), gjson.GetBytes(body, "models.0.max_output_tokens").Int())
	require.Equal(t, "[]", gjson.GetBytes(body, "models.0.input_modalities").Raw)
	require.Equal(t, int64(500000), gjson.GetBytes(body, "models.1.context_window").Int())
}
func BenchmarkCodexCatalogProjection(b *testing.B) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	manifest := &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":777777,"input_modalities":[]}]}`)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCodexCachedAdjustmentsDoNotOverrideStoredExplicitCapabilities(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{modelcatalog.ExtraKey: catalogMetadataFixture(t, 600000)}}
	raw := []byte(`{"models":[{"slug":"gpt-6.1-sol"}]}`)
	adjusted, err := adjustAPIKeyCodexModelsManifest(raw)
	require.NoError(t, err)
	manifest := &CodexModelsManifest{Body: adjusted, rawBody: raw, apiKeyUpstream: true}
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, "[]", gjson.GetBytes(projected.Body, "models.0.input_modalities").Raw)
	require.Equal(t, int64(600000), gjson.GetBytes(projected.Body, "models.0.context_window").Int())
}
