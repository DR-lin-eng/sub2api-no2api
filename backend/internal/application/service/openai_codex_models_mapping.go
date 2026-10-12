package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// CodexManifestMappingEnabled limits projection to stored administrator mappings.
// Default OAuth aliases and explicit passthrough retain the existing manifest policy.
func CodexManifestMappingEnabled(account *Account) bool {
	if account == nil || account.IsOpenAIPassthroughEnabled() {
		return false
	}
	switch mapping := account.Credentials["model_mapping"].(type) {
	case map[string]any:
		return len(mapping) > 0
	case map[string]string:
		return len(mapping) > 0
	}
	return false
}

func codexMappingCatalogEntries(body []byte) (map[string]json.RawMessage, []json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &entries); err != nil || entries == nil {
		return nil, nil, fmt.Errorf("invalid Codex models array")
	}
	return envelope, entries, nil
}

func projectCodexAccountModelsBody(body []byte, account *Account, group *Group) ([]byte, error) {
	if !CodexManifestMappingEnabled(account) {
		return body, nil
	}
	field, idField := "models", "slug"
	envelope, entries, err := codexMappingCatalogEntries(body)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]json.RawMessage, len(entries))
	candidates := make([]string, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}
		var id string
		if json.Unmarshal(entry[idField], &id) != nil {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "*") {
			continue
		}
		if _, ok := byID[id]; !ok {
			byID[id] = raw
			candidates = append(candidates, id)
		}
	}
	aliases := make([]string, 0, len(account.GetModelMapping()))
	for alias := range account.GetModelMapping() {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	candidates = append(candidates, aliases...)
	if group.ModelAllowlistEnabled() {
		candidates = append(candidates, group.ModelAllowlist.Models...)
	}
	projected := make([]json.RawMessage, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "*") {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		target, matched := account.ResolveMappedModel(id)
		raw, available := byID[strings.TrimSpace(target)]
		if !matched || !available {
			continue
		}
		seen[id] = struct{}{}
		if id == target {
			projected = append(projected, raw)
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		entry[idField], _ = json.Marshal(id)
		if id != target {
			entry["display_name"], _ = json.Marshal(id)
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		projected = append(projected, encoded)
	}
	if slices.EqualFunc(entries, projected, func(a, b json.RawMessage) bool { return bytes.Equal(a, b) }) {
		return body, nil
	}
	envelope[field], err = json.Marshal(projected)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func ProjectCodexModelsManifestForAccount(manifest *CodexModelsManifest, account *Account, group *Group, ifNoneMatch string) (*CodexModelsManifest, error) {
	if manifest == nil || manifest.NotModified || !CodexManifestMappingEnabled(account) {
		return manifest, nil
	}
	body, err := projectCodexAccountModelsBody(manifest.Body, account, group)
	if err != nil {
		return nil, err
	}
	projected := *manifest
	if !bytes.Equal(body, manifest.Body) {
		projected.Body = body
		projected.ETag = codexModelsManifestBodyETag(body)
	}
	return codexModelsManifestForClient(&projected, ifNoneMatch), nil
}
