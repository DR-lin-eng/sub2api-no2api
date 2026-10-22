// Package modelcatalog owns bounded provider-neutral discovery snapshots.
package modelcatalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	Version          = 1
	ExtraKey         = "upstream_model_metadata"
	MaxModels        = 2048
	MaxBodyBytes     = 8 << 20
	MaxSnapshotBytes = 1 << 20
)

// Descriptor retains field presence: false and empty arrays are explicit values.
// Only validated capability fields can enter a persisted descriptor.
type Descriptor map[string]json.RawMessage

type Catalog struct {
	Models   []string              `json:"models"`
	Metadata map[string]Descriptor `json:"metadata,omitempty"`
}

type Snapshot struct {
	Version    int                   `json:"version"`
	Source     string                `json:"source"`
	StartedAt  time.Time             `json:"started_at"`
	Generation int64                 `json:"generation"`
	SyncedAt   time.Time             `json:"synced_at"`
	Models     map[string]Descriptor `json:"models"`
}

func ValidID(id string) bool {
	if id == "" || len(id) > 256 || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '*' {
			return false
		}
	}
	return true
}

func (c *Catalog) Snapshot(source string, started time.Time) (*Snapshot, error) {
	if c == nil || len(c.Models) == 0 || len(c.Models) > MaxModels {
		return nil, fmt.Errorf("invalid catalog model count")
	}
	out := &Snapshot{Version: Version, Source: source, StartedAt: started.UTC(), Generation: started.UnixMicro(), SyncedAt: time.Now().UTC(), Models: make(map[string]Descriptor, len(c.Models))}
	for _, id := range c.Models {
		if !ValidID(id) {
			return nil, fmt.Errorf("invalid model ID")
		}
		out.Models[id] = Normalize(c.Metadata[id], id)
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > MaxSnapshotBytes {
		return nil, fmt.Errorf("model metadata snapshot exceeds limit")
	}
	return out, nil
}

func ReadSnapshot(extra map[string]any) *Snapshot {
	raw, err := json.Marshal(extra[ExtraKey])
	if err != nil || len(raw) > MaxSnapshotBytes {
		return nil
	}
	var snapshot Snapshot
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Version != Version || len(snapshot.Models) > MaxModels {
		return nil
	}
	// Revalidate persisted input, including records created by an older writer.
	for id, d := range snapshot.Models {
		if !ValidID(id) {
			return nil
		}
		snapshot.Models[id] = Normalize(d, id)
	}
	return &snapshot
}

// Parse reads OpenAI, Codex, Anthropic, Gemini and keyed Antigravity catalogs.
// IDs are supplied by the transport's existing provider-specific selector, so
// Grok _meta IDs and other account routing semantics stay unchanged.
func Parse(body []byte, ids []string) (map[string]Descriptor, error) {
	if len(body) > MaxBodyBytes || len(ids) > MaxModels {
		return nil, fmt.Errorf("model catalog exceeds limit")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		var array []json.RawMessage
		if json.Unmarshal(body, &array) != nil {
			return nil, err
		}
		envelope = map[string]json.RawMessage{"data": body}
	}
	wanted := make(map[string]bool, len(ids))
	out := make(map[string]Descriptor, len(ids))
	for _, id := range ids {
		if !ValidID(id) {
			return nil, fmt.Errorf("invalid model ID")
		}
		wanted[id] = true
	}
	raw, ok := envelope["data"]
	if !ok {
		raw = envelope["models"]
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, fmt.Errorf("invalid model catalog entries")
		}
		keys := make([]string, 0, len(keyed))
		for id := range keyed {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			var d Descriptor
			if json.Unmarshal(keyed[id], &d) != nil || d == nil {
				return nil, fmt.Errorf("invalid model descriptor")
			}
			d["id"], _ = json.Marshal(id)
			encoded, _ := json.Marshal(d)
			entries = append(entries, encoded)
		}
	}
	if len(entries) > MaxModels {
		return nil, fmt.Errorf("model catalog exceeds limit")
	}
	for _, raw := range entries {
		var d Descriptor
		if json.Unmarshal(raw, &d) != nil || d == nil {
			return nil, fmt.Errorf("invalid model descriptor")
		}
		candidates := []string{}
		for _, key := range []string{"id", "slug", "model", "modelId", "model_id", "name"} {
			var id string
			_ = json.Unmarshal(d[key], &id)
			id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
			if id != "" {
				candidates = append(candidates, id)
			}
		}
		var meta map[string]json.RawMessage
		if json.Unmarshal(d["_meta"], &meta) == nil {
			for _, key := range []string{"id", "model", "modelId", "model_id", "name"} {
				var id string
				_ = json.Unmarshal(meta[key], &id)
				candidates = append(candidates, strings.TrimSpace(id))
			}
		}
		for _, id := range candidates {
			if wanted[id] {
				if previous, duplicate := out[id]; duplicate {
					a, _ := json.Marshal(previous)
					b, _ := json.Marshal(Normalize(d, id))
					if string(a) != string(b) {
						return nil, fmt.Errorf("conflicting model descriptor")
					}
					break
				}
				out[id] = Normalize(d, id)
				break
			}
		}
	}
	for _, id := range ids {
		if out[id] == nil {
			return nil, fmt.Errorf("model descriptor missing from catalog")
		}
	}
	return out, nil
}

// Fill applies fallbacks only to absent fields, never explicit false/empty/null.
func Fill(primary, fallback Descriptor) Descriptor {
	out := make(Descriptor, len(primary)+len(fallback))
	for k, v := range fallback {
		out[k] = append(json.RawMessage(nil), v...)
	}
	for k, v := range primary {
		out[k] = append(json.RawMessage(nil), v...)
	}
	if string(primary["supports_reasoning"]) == "false" {
		for _, key := range []string{"supported_reasoning_levels", "default_reasoning_level"} {
			if _, ok := primary[key]; !ok {
				delete(out, key)
			}
		}
	}
	return out
}
