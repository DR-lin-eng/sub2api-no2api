package modelcatalog

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGrokCatalogDiagnosticsTrustAndNoMutation(t *testing.T) {
	now := time.Now().UTC()
	mapping := map[string]string{"listed": "xai/grok-4.6-latest", "missing": "grok-4.5", "bare": "grok", "wild*": "grok-4.5", "media": "grok-imagine-image", "custom": "custom-private", "build": "grok-4.6-build"}
	for _, tc := range []struct {
		name, scope string
		age         time.Duration
		want        string
	}{{"fresh", "scope", time.Minute, "fresh"}, {"stale", "scope", GrokCatalogTTL, "stale"}, {"old", "", time.Minute, "unknown"}, {"changed", "elsewhere", time.Minute, "source_changed"}, {"future", "scope", -time.Minute, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &Snapshot{Version: 1, DiscoveryScope: tc.scope, SyncedAt: now.Add(-tc.age), Models: map[string]Descriptor{"grok-4.6": {}}}
			before, _ := json.Marshal(snapshot)
			d := EvaluateGrok(snapshot, "scope", mapping, now)
			require.Equal(t, tc.want, d.State)
			status := map[string]string{}
			for _, item := range d.Mappings {
				status[item.Requested] = item.Status
			}
			require.Equal(t, "out_of_scope", status["media"])
			for _, key := range []string{"bare", "wild*", "custom", "build"} {
				require.Equal(t, "unverifiable", status[key])
			}
			if tc.want == "fresh" {
				require.Equal(t, "listed", status["listed"])
				require.Equal(t, "not_listed", status["missing"])
			}
			if tc.want == "stale" {
				require.Equal(t, "listed", status["listed"])
				require.Equal(t, "unverifiable", status["missing"])
			}
			if tc.want == "unknown" || tc.want == "source_changed" {
				require.Equal(t, "unverifiable", status["listed"])
			}
			after, _ := json.Marshal(snapshot)
			require.Equal(t, string(before), string(after))
		})
	}
}
func TestGrokCatalogStringAndDualListsMetadata(t *testing.T) {
	catalog, err := Parse([]byte(`{"data":["grok-4.6"],"models":[{"id":"grok-4.5","context_window":999}]}`), []string{"grok-4.6", "grok-4.5"})
	require.NoError(t, err)
	require.Contains(t, catalog, "grok-4.6")
	require.Equal(t, "999", string(catalog["grok-4.5"]["context_window"]))
}

func TestGrokCatalogMixedDuplicateDoesNotEraseExplicitMetadata(t *testing.T) {
	for _, raw := range []string{`{"data":[{"id":"grok-4.6","supports_reasoning":false},"grok-4.6"]}`, `{"data":["grok-4.6",{"id":"grok-4.6","supports_reasoning":false}]}`} {
		_, err := Parse([]byte(raw), []string{"grok-4.6"})
		require.ErrorContains(t, err, "conflicting model descriptor")
	}
	descriptor, err := Parse([]byte(`{"data":[" grok-4.6 ","grok-4.6"]}`), []string{"grok-4.6"})
	require.NoError(t, err)
	require.Equal(t, `"grok-4.6"`, string(descriptor["grok-4.6"]["id"]))
}
