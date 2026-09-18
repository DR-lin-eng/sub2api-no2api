package service

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

const (
	codexWorkspaceMetadataMaxDepth = 5
	codexWorkspaceMetadataMaxBytes = 8192
)

// sanitizeCodexTurnMetadataValue keeps the official workspace projection
// useful for upstream risk evaluation while removing local paths and remote
// credential material. Invalid metadata is left untouched so the existing
// protocol error handling remains authoritative.
func sanitizeCodexTurnMetadataValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return raw
	}
	value, changed, keep := sanitizedCodexTurnMetadataMap(metadata)
	if !changed {
		return raw
	}
	if !keep {
		return `{"redacted":true}`
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > codexWorkspaceMetadataMaxBytes {
		return `{"redacted":true}`
	}
	return string(encoded)
}

func sanitizedCodexTurnMetadataMap(metadata map[string]any) (map[string]any, bool, bool) {
	value, changed, keep := sanitizeCodexWorkspaceValue(metadata, "", 0, false)
	if !keep {
		return nil, true, false
	}
	sanitized, ok := value.(map[string]any)
	if !ok {
		return metadata, changed, true
	}
	return sanitized, changed, true
}

func sanitizeCodexTurnMetadataMapInPlace(metadata map[string]any) {
	if metadata == nil {
		return
	}
	sanitized, changed, keep := sanitizedCodexTurnMetadataMap(metadata)
	if !changed {
		return
	}
	for key := range metadata {
		delete(metadata, key)
	}
	if !keep {
		metadata["redacted"] = true
		return
	}
	for key, value := range sanitized {
		metadata[key] = value
	}
}

func sanitizeCodexWorkspaceValue(value any, key string, depth int, inWorkspace bool) (any, bool, bool) {
	if depth > codexWorkspaceMetadataMaxDepth {
		return nil, true, false
	}

	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		changed := false
		for _, childKey := range sortedCodexWorkspaceKeys(typed) {
			child := typed[childKey]
			childInWorkspace := inWorkspace || codexWorkspaceContainerKey(childKey)
			if childInWorkspace && codexWorkspaceSensitiveKey(childKey) {
				changed = true
				continue
			}
			sanitized, childChanged, keep := sanitizeCodexWorkspaceValue(child, childKey, depth+1, childInWorkspace)
			if childChanged {
				changed = true
			}
			if keep {
				out[childKey] = sanitized
			} else {
				changed = true
			}
		}
		return out, changed, true
	case []any:
		out := make([]any, 0, len(typed))
		changed := false
		for _, child := range typed {
			sanitized, childChanged, keep := sanitizeCodexWorkspaceValue(child, key, depth+1, inWorkspace)
			changed = changed || childChanged
			if keep {
				out = append(out, sanitized)
			} else {
				changed = true
			}
		}
		return out, changed, true
	case string:
		if (inWorkspace && codexWorkspacePathKey(key)) || codexRootWorkspacePathKey(key) {
			if strings.TrimSpace(typed) == "" {
				return typed, false, true
			}
			return stableCodexWorkspacePath(typed), true, true
		}
		if inWorkspace && codexWorkspaceRemoteKey(key) {
			sanitized, changed := sanitizeCodexRemoteURL(typed)
			return sanitized, changed, true
		}
		return typed, false, true
	default:
		return value, false, true
	}
}

func codexWorkspaceContainerKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "workspace", "workspaces", "workspace_info", "workspace_metadata":
		return true
	default:
		return false
	}
}

func sortedCodexWorkspaceKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func codexWorkspaceSensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"token", "secret", "password", "credential", "authorization", "cookie"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func codexWorkspacePathKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "path", "cwd", "workspace_path", "absolute_path", "worktree", "git_dir", "root", "root_path":
		return true
	default:
		return false
	}
}

func codexRootWorkspacePathKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "cwd", "workspace_path", "absolute_path", "worktree", "root_path":
		return true
	default:
		return false
	}
}

func codexWorkspaceRemoteKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "remote") || strings.Contains(lower, "url") || lower == "origin"
}

func stableCodexWorkspacePath(_ string) string {
	return "workspace:redacted"
}

func sanitizeCodexRemoteURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, false
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		changed := parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != ""
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String(), changed
	}
	// Git's scp-like form has no URL parser userinfo field. Remove the user
	// portion while retaining the host and repository path for coarse matching.
	if at := strings.IndexByte(raw, '@'); at > 0 {
		if colon := strings.IndexByte(raw[at+1:], ':'); colon >= 0 {
			return raw[at+1:], true
		}
	}
	return raw, false
}
