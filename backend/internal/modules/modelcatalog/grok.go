package modelcatalog

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	"sort"
	"strings"
	"time"
)

const GrokCatalogTTL = 6 * time.Hour

type GrokMappingDiagnostic struct {
	Requested string `json:"requested"`
	Upstream  string `json:"upstream,omitempty"`
	Status    string `json:"status"`
}
type GrokDiagnostics struct {
	State     string                  `json:"state"`
	FetchedAt string                  `json:"fetched_at,omitempty"`
	ExpiresAt string                  `json:"expires_at,omitempty"`
	Models    []string                `json:"models"`
	Mappings  []GrokMappingDiagnostic `json:"mappings"`
}

// EvaluateGrok describes saved catalog evidence. It never changes routing,
// pricing, allowlists, or the snapshot. An old snapshot without scope is unknown.
func EvaluateGrok(snapshot *Snapshot, scope string, mapping map[string]string, now time.Time) *GrokDiagnostics {
	result := &GrokDiagnostics{State: "unknown", Models: []string{}, Mappings: []GrokMappingDiagnostic{}}
	catalog := map[string]bool{}
	if snapshot != nil {
		for id := range snapshot.Models {
			result.Models = append(result.Models, id)
			catalog[grokCatalogID(id)] = true
		}
		sort.Strings(result.Models)
		if !snapshot.SyncedAt.IsZero() && !snapshot.SyncedAt.After(now) {
			result.FetchedAt = snapshot.SyncedAt.UTC().Format(time.RFC3339Nano)
			result.ExpiresAt = snapshot.SyncedAt.Add(GrokCatalogTTL).UTC().Format(time.RFC3339Nano)
			if scope != "" && snapshot.DiscoveryScope != "" && len(snapshot.Models) > 0 {
				switch {
				case scope != snapshot.DiscoveryScope:
					result.State = "source_changed"
				case now.Sub(snapshot.SyncedAt) >= GrokCatalogTTL:
					result.State = "stale"
				default:
					result.State = "fresh"
				}
			}
		}
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		target := strings.TrimSpace(mapping[key])
		item := GrokMappingDiagnostic{Requested: key, Upstream: grokCatalogID(target), Status: "unverifiable"}
		lower := strings.ToLower(xai.StripGrokProviderPrefix(target))
		switch {
		case strings.Contains(key, "*") || strings.Contains(target, "*") || target == "":
		case independentGrokCapability(lower):
			item.Status = "out_of_scope"
		case lower == "grok" || lower == "grok-latest":
		case (result.State == "fresh" || result.State == "stale") && catalog[item.Upstream]:
			item.Status = "listed"
		case result.State == "fresh" && knownGrokTextModel(target):
			item.Status = "not_listed"
		}
		result.Mappings = append(result.Mappings, item)
	}
	return result
}
func grokCatalogID(id string) string {
	id = strings.TrimSpace(id)
	bare := strings.ToLower(xai.StripGrokProviderPrefix(id))
	if bare == "grok" || bare == "grok-latest" {
		return id
	}
	return strings.TrimSpace(xai.ResolveGrokTextResponsesModelID(id))
}
func independentGrokCapability(id string) bool {
	if strings.HasPrefix(id, "grok-imagine") {
		return true
	}
	for _, term := range []string{"voice", "realtime", "audio", "video", "tts", "stt"} {
		if strings.Contains(id, term) {
			return true
		}
	}
	return false
}

var grokTextModelIDs = func() map[string]bool {
	out := map[string]bool{}
	for _, id := range xai.DefaultModelIDs() {
		if !independentGrokCapability(id) {
			out[id] = true
		}
	}
	return out
}()

func knownGrokTextModel(model string) bool { return grokTextModelIDs[grokCatalogID(model)] }
