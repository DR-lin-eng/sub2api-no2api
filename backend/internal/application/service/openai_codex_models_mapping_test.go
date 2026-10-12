package service

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexManifestAccountMappingPreservesMetadataAndCache(t *testing.T) {
	body := []byte(`{"revision":"fixture","models":[{"slug":"target","future":{"keep":true},"service_tiers":[]},{"slug":"other"}]}`)
	account := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"alias": "target"}}}
	manifest := &CodexModelsManifest{Body: body, ETag: `"original"`}
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.JSONEq(t, `{"revision":"fixture","models":[{"slug":"alias","display_name":"alias","future":{"keep":true},"service_tiers":[]}]}`, string(projected.Body))
	require.Equal(t, body, manifest.Body)
	require.Equal(t, `"original"`, manifest.ETag)
	require.NotEqual(t, manifest.ETag, projected.ETag)
	conditional, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, projected.ETag)
	require.NoError(t, err)
	require.True(t, conditional.NotModified)
}

func TestCodexManifestIdentityMappingKeepsExactBytesAndETag(t *testing.T) {
	body := []byte("{\n \"models\": [ {\"slug\":\"target\",\"future\":true} ]\n}")
	account := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"target": "target"}}}
	manifest := &CodexModelsManifest{Body: body, ETag: `"original"`}
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, projected.Body))
	require.Equal(t, manifest.ETag, projected.ETag)
}

func TestCodexManifestNullServiceTiersUsesEmptyArray(t *testing.T) {
	out, err := adjustAPIKeyCodexModelsManifest([]byte(`{"models":[{"slug":"custom","service_tiers":null,"future":"kept"}]}`))
	require.NoError(t, err)
	var envelope struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	require.NoError(t, json.Unmarshal(out, &envelope))
	require.Equal(t, "[]", string(envelope.Models[0]["service_tiers"]))
	require.Equal(t, `"kept"`, string(envelope.Models[0]["future"]))
}
