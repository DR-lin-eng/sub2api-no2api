package service

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
)

// CodexManifestProjectionEnabled requires local conditional validation whenever
// account metadata may affect the body, independently of upstream cache ETags.
func CodexManifestProjectionEnabled(account *Account) bool {
	return account != nil && !account.IsOpenAIPassthroughEnabled()
}

func codexMetadataOverrides(account *Account) map[string]modelcatalog.Descriptor {
	if account == nil {
		return nil
	}
	raw, err := json.Marshal(account.Extra["model_metadata_overrides"])
	if err != nil || len(raw) > modelcatalog.MaxSnapshotBytes {
		return nil
	}
	var out map[string]modelcatalog.Descriptor
	if json.Unmarshal(raw, &out) != nil || len(out) > modelcatalog.MaxModels {
		return nil
	}
	for id, d := range out {
		if !modelcatalog.ValidID(id) {
			return nil
		}
		out[id] = modelcatalog.Normalize(d, id)
	}
	return out
}

func applyCodexMetadataOverride(entry modelcatalog.Descriptor, override modelcatalog.Descriptor) modelcatalog.Descriptor {
	for key, value := range override {
		if key != "id" && key != "slug" {
			entry[key] = value
		}
	}
	return entry
}

func enrichCodexModelsManifest(body []byte, account *Account) ([]byte, error) {
	envelope, entries, err := codexMappingCatalogEntries(body)
	if err != nil {
		return nil, err
	}
	if len(entries) > modelcatalog.MaxModels {
		return nil, fmt.Errorf("codex model count exceeds limit")
	}
	snapshot := modelcatalog.ReadSnapshot(nil)
	if account != nil {
		snapshot = modelcatalog.ReadSnapshot(account.Extra)
	}
	overrides := codexMetadataOverrides(account)
	changed := false
	for i, raw := range entries {
		var entry modelcatalog.Descriptor
		if json.Unmarshal(raw, &entry) != nil || entry == nil {
			continue
		}
		var id string
		if json.Unmarshal(entry["slug"], &id) != nil || !modelcatalog.ValidID(id) {
			continue
		}
		fallback := modelcatalog.KnownFallback(id)
		if snapshot != nil {
			fallback = modelcatalog.Fill(snapshot.Models[id], fallback)
		}
		merged := modelcatalog.Fill(entry, fallback)
		delete(merged, "id")
		merged = applyCodexMetadataOverride(merged, overrides[id])
		normalizeCodexReasoningLevels(merged)
		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		// Compare decoded content, avoiding unnecessary ETag changes for whitespace.
		old, _ := json.Marshal(entry)
		if !bytes.Equal(old, encoded) {
			entries[i] = encoded
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	envelope["models"], err = json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func normalizeCodexReasoningLevels(entry modelcatalog.Descriptor) {
	raw, ok := entry["supported_reasoning_levels"]
	if !ok {
		return
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return
	}
	for i, value := range values {
		var effort string
		if json.Unmarshal(value, &effort) == nil {
			values[i], _ = json.Marshal(map[string]string{"effort": effort, "description": effort})
		}
	}
	if values != nil {
		entry["supported_reasoning_levels"], _ = json.Marshal(values)
	}
}
