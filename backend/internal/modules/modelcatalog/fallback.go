package modelcatalog

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
)

// Generated from the reviewed repository price resource by
// backend/scripts/update-model-catalog.py. Never fetched on a request path.
//
//go:embed fallbacks.json
var fallbackJSON []byte
var fallbackOnce sync.Once
var fallbackCatalog map[string]Descriptor

// KnownFallback returns a copy of the compiled descriptor. Explicit fields
// from an upstream or administrator always take precedence over this catalog.
func KnownFallback(id string) Descriptor {
	fallbackOnce.Do(func() {
		var catalog map[string]Descriptor
		if json.Unmarshal(fallbackJSON, &catalog) == nil {
			fallbackCatalog = make(map[string]Descriptor, len(catalog))
			for id, d := range catalog {
				if ValidID(id) {
					fallbackCatalog[id] = Normalize(d, id)
				}
			}
		}
	})
	name := strings.ToLower(strings.TrimSpace(id))
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	if _, exists := fallbackCatalog[name]; !exists {
		switch {
		case claude.IsOpus55(name):
			name = "claude-opus-5-5"
		case claude.IsSonnet55(name):
			name = "claude-sonnet-5-5"
		case claude.IsHaiku55(name):
			name = "claude-haiku-5-5"
		default:
			if len(name) > 11 && name[len(name)-11] == '-' {
				suffix := name[len(name)-10:]
				valid := true
				for i, c := range suffix {
					if i == 4 || i == 7 {
						if c != '-' {
							valid = false
						}
					} else if c < '0' || c > '9' {
						valid = false
					}
				}
				if valid {
					name = name[:len(name)-11]
				}
			}
		}
	}
	return Fill(nil, fallbackCatalog[name])
}
