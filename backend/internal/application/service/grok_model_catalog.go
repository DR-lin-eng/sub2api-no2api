package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Grok parsing rejects ambiguous success/error envelopes and incomplete entries.
func parseGrokCatalogModelIDs(body []byte) ([]string, error) {
	if len(body) > modelcatalog.MaxBodyBytes {
		return nil, fmt.Errorf("grok catalog exceeds limit")
	}
	raw := bytes.TrimSpace(body)
	var entries []json.RawMessage
	if len(raw) == 0 {
		return nil, fmt.Errorf("invalid grok catalog")
	}
	switch raw[0] {
	case '[':
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
	case '{':
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, err
		}
		if value, ok := envelope["error"]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("grok catalog error response")
		}
		found := false
		for _, key := range []string{"data", "models"} {
			if value, ok := envelope[key]; ok {
				found = true
				value = bytes.TrimSpace(value)
				if len(value) == 0 || value[0] != '[' {
					return nil, fmt.Errorf("invalid grok catalog entries")
				}
				var items []json.RawMessage
				if err := json.Unmarshal(value, &items); err != nil {
					return nil, err
				}
				entries = append(entries, items...)
			}
		}
		if !found {
			return nil, fmt.Errorf("missing grok catalog")
		}
	default:
		return nil, fmt.Errorf("invalid grok catalog")
	}
	if len(entries) == 0 || len(entries) > modelcatalog.MaxModels {
		return nil, fmt.Errorf("invalid grok catalog model count")
	}
	ids := make([]string, 0, len(entries))
	for _, raw := range entries {
		var id string
		if json.Unmarshal(raw, &id) != nil {
			var entry upstreamModelEntry
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return nil, fmt.Errorf("invalid grok catalog entry")
			}
			if err := json.Unmarshal(raw, &entry); err != nil {
				return nil, err
			}
			id = grokUpstreamModelEntryID(entry)
		}
		id = strings.TrimSpace(id)
		if !modelcatalog.ValidID(id) {
			return nil, fmt.Errorf("invalid grok model ID")
		}
		ids = append(ids, id)
	}
	return dedupeAndSortModelIDs(ids), nil
}
func grokModelsScope(account *Account, endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.Fragment = ""
	parsed.User = nil
	sum := sha256.Sum256([]byte(strconv.FormatInt(account.ID, 10) + "\n" + account.Type + "\n" + parsed.String()))
	return hex.EncodeToString(sum[:])
}

// No token refresh, HTTP, DNS lookup, database query, or scheduling mutation.
func GetGrokModelDiagnostics(account *Account) *modelcatalog.GrokDiagnostics {
	if account == nil || !account.IsGrok() {
		return nil
	}
	base := account.GetGrokBaseURL()
	return modelcatalog.EvaluateGrok(modelcatalog.ReadSnapshot(account.Extra), grokModelsScope(account, buildOpenAIModelsURL(base)), account.GetModelMapping(), time.Now())
}
