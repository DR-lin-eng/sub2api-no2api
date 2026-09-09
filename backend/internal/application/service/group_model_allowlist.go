package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// GroupModelAllowlist is the group-level model policy exposed by the admin API.
// Entries are exact model IDs or a prefix ending in '*'.
type GroupModelAllowlist struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}

func DomainGroupModelAllowlist(cfg GroupModelAllowlist) domain.GroupModelAllowlist {
	return domain.GroupModelAllowlist{Enabled: cfg.Enabled, Models: append([]string(nil), cfg.Models...)}
}

func GroupModelAllowlistFromDomain(cfg domain.GroupModelAllowlist) GroupModelAllowlist {
	return GroupModelAllowlist{Enabled: cfg.Enabled, Models: append([]string(nil), cfg.Models...)}
}

// normalizeGroupModelAllowlist trims, de-duplicates and validates admin input.
func normalizeGroupModelAllowlist(cfg GroupModelAllowlist) (GroupModelAllowlist, error) {
	out := GroupModelAllowlist{Enabled: cfg.Enabled}
	seen := make(map[string]struct{}, len(cfg.Models))
	for _, raw := range cfg.Models {
		model := strings.TrimSpace(raw)
		if model == "" {
			continue
		}
		if strings.Contains(strings.TrimSuffix(model, "*"), "*") {
			return out, infraerrors.New(http.StatusBadRequest, "INVALID_MODEL_ALLOWLIST", `wildcard "*" is only allowed at the end of an allowlist entry`)
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out.Models = append(out.Models, model)
	}
	if cfg.Enabled && len(out.Models) == 0 {
		return out, infraerrors.New(http.StatusBadRequest, "INVALID_MODEL_ALLOWLIST", "model allowlist cannot be enabled with an empty model list")
	}
	return out, nil
}

func (g *Group) ModelAllowlistEnabled() bool {
	return g != nil && g.ModelAllowlist.Enabled
}

// Allows checks the client-visible model name before routing or mapping.
func (a GroupModelAllowlist) Allows(model string) bool {
	if !a.Enabled || strings.TrimSpace(model) == "" {
		return true
	}
	candidates := groupModelAllowlistCandidates(model)
	for _, raw := range a.Models {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			continue
		}
		if strings.HasSuffix(entry, "*") {
			prefix := strings.TrimSuffix(entry, "*")
			for _, candidate := range candidates {
				if strings.HasPrefix(candidate, prefix) {
					return true
				}
			}
			continue
		}
		for _, candidate := range candidates {
			if candidate == entry {
				return true
			}
		}
	}
	return false
}

func groupModelAllowlistCandidates(model string) []string {
	var candidates []string
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}
	add(model)
	add(strings.TrimPrefix(model, "models/"))
	add(claude.NormalizeModelID(strings.TrimSuffix(model, "-thinking")))
	add(NormalizeOpenAICompatRequestedModel(model))
	return candidates
}

// FilterForListing returns source entries that are covered by the allowlist,
// preserving configured allowlist order and source spelling for wildcard rows.
func (a GroupModelAllowlist) FilterForListing(source []string) []string {
	if !a.Enabled {
		return source
	}
	seen := make(map[string]struct{}, len(source))
	result := make([]string, 0, len(a.Models))
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	for _, raw := range a.Models {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.HasSuffix(entry, "*") {
			prefix := strings.ToLower(strings.TrimSuffix(entry, "*"))
			for _, candidate := range source {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(candidate)), prefix) {
					add(candidate)
				}
			}
			continue
		}
		for _, candidate := range source {
			if allowlistSourcePatternAllowsModel(candidate, entry) ||
				(GroupModelAllowlist{Enabled: true, Models: []string{entry}}).Allows(candidate) {
				add(entry)
				break
			}
		}
	}
	return result
}

func allowlistSourcePatternAllowsModel(source, model string) bool {
	source = strings.TrimSpace(source)
	model = strings.TrimSpace(model)
	if strings.EqualFold(source, model) {
		return true
	}
	if strings.HasSuffix(source, "*") && strings.HasPrefix(strings.ToLower(model), strings.ToLower(strings.TrimSuffix(source, "*"))) {
		return true
	}
	normalized := claude.NormalizeModelID(strings.TrimSuffix(model, "-thinking"))
	return strings.EqualFold(source, normalized)
}
