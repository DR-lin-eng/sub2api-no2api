package handler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/websearch"
	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	"github.com/tidwall/gjson"
)

const (
	defaultGrokXSearchResults = 5
	maxGrokXSearchResults     = 20
)

type grokXSearchRequest struct {
	Query                    string   `json:"query"`
	Input                    string   `json:"input"`
	MaxResults               *int     `json:"max_results"`
	AllowedXHandles          []string `json:"allowed_x_handles"`
	ExcludedXHandles         []string `json:"excluded_x_handles"`
	FromDate                 string   `json:"from_date"`
	ToDate                   string   `json:"to_date"`
	EnableImageUnderstanding *bool    `json:"enable_image_understanding"`
	EnableVideoUnderstanding *bool    `json:"enable_video_understanding"`
}

func buildGrokXSearchResponsesBody(req grokXSearchRequest, model string, maxResults int) ([]byte, error) {
	tool := map[string]any{"type": "x_search"}
	if len(req.AllowedXHandles) > 0 {
		tool["allowed_x_handles"] = req.AllowedXHandles
	}
	if len(req.ExcludedXHandles) > 0 {
		tool["excluded_x_handles"] = req.ExcludedXHandles
	}
	if fromDate := strings.TrimSpace(req.FromDate); fromDate != "" {
		tool["from_date"] = fromDate
	}
	if toDate := strings.TrimSpace(req.ToDate); toDate != "" {
		tool["to_date"] = toDate
	}
	if req.EnableImageUnderstanding != nil {
		tool["enable_image_understanding"] = *req.EnableImageUnderstanding
	}
	if req.EnableVideoUnderstanding != nil {
		tool["enable_video_understanding"] = *req.EnableVideoUnderstanding
	}
	return json.Marshal(map[string]any{
		"model":       xai.ResolveGrokTextResponsesModelID(model),
		"input":       buildGrokXSearchPrompt(req.Query, maxResults),
		"tools":       []map[string]any{tool},
		"tool_choice": "required",
		"include":     []string{"x_search_call.action.sources"},
		"store":       false,
		"stream":      false,
	})
}

func buildGrokXSearchPrompt(query string, maxResults int) string {
	return fmt.Sprintf(`Search X for the user query below. Return ONLY valid JSON with this exact shape: {"results":[{"url":"https://...","title":"post or page title","snippet":"concise factual summary"}]}. Return at most %d unique results. Every URL must be an actual x_search source. Populate a non-empty title and snippet for every result. Do not wrap the JSON in markdown.

User query:
%s`, normalizeGrokXSearchMaxResults(maxResults), query)
}

func normalizeGrokXSearchMaxResults(maxResults int) int {
	if maxResults <= 0 {
		return defaultGrokXSearchResults
	}
	if maxResults > maxGrokXSearchResults {
		return maxGrokXSearchResults
	}
	return maxResults
}

// extractGrokSearchSources accepts model-enriched results only when the URL is
// present in actual x_search sources (or a response citation fallback).
func extractGrokSearchSources(body []byte, maxResults int) []websearch.SearchResult {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	maxResults = normalizeGrokXSearchMaxResults(maxResults)
	sources := make(map[string]websearch.SearchResult)
	sourceOrder := make([]string, 0)
	addSource := func(rawURL, title, snippet string) {
		key, ok := normalizeGrokSearchURL(rawURL)
		if !ok {
			return
		}
		result, exists := sources[key]
		if !exists {
			result.URL = strings.TrimSpace(rawURL)
			sourceOrder = append(sourceOrder, key)
		}
		if result.Title == "" {
			result.Title = usableGrokSearchTitle(title, result.URL)
		}
		if result.Snippet == "" {
			result.Snippet = strings.TrimSpace(snippet)
		}
		sources[key] = result
	}

	output := gjson.GetBytes(body, "output")
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "x_search_call" {
			item.Get("action.sources").ForEach(func(_, src gjson.Result) bool {
				addSource(src.Get("url").String(), src.Get("title").String(), src.Get("snippet").String())
				return true
			})
		}
		if item.Get("type").String() == "message" {
			item.Get("content").ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").String() != "output_text" {
					return true
				}
				part.Get("annotations").ForEach(func(_, ann gjson.Result) bool {
					if ann.Get("type").String() == "url_citation" || ann.Get("type").String() == "web" {
						addSource(ann.Get("url").String(), ann.Get("title").String(), "")
					}
					return true
				})
				return true
			})
		}
		return true
	})

	out := make([]websearch.SearchResult, 0, min(maxResults, len(sources)))
	seen := make(map[string]bool, len(sources))
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "message" {
			return true
		}
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() != "output_text" || len(out) >= maxResults {
				return true
			}
			for _, result := range parseGrokSearchStructuredResults(part.Get("text").String()) {
				key, ok := normalizeGrokSearchURL(result.URL)
				if !ok || seen[key] {
					continue
				}
				source, allowed := sources[key]
				if !allowed {
					continue
				}
				seen[key] = true
				result.URL = source.URL
				result.Title = usableGrokSearchTitle(result.Title, result.URL)
				if result.Title == "" {
					result.Title = source.Title
				}
				result.Snippet = strings.TrimSpace(result.Snippet)
				if result.Snippet == "" {
					result.Snippet = source.Snippet
				}
				out = append(out, result)
				if len(out) >= maxResults {
					break
				}
			}
			return true
		})
		return len(out) < maxResults
	})
	for _, key := range sourceOrder {
		if len(out) >= maxResults {
			break
		}
		if seen[key] {
			continue
		}
		result := sources[key]
		if result.Title == "" {
			result.Title = grokSearchTitleFromURL(result.URL)
		}
		seen[key] = true
		out = append(out, result)
	}
	return out
}

func parseGrokSearchStructuredResults(text string) []websearch.SearchResult {
	text = strings.TrimSpace(text)
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return nil
	}
	var payload struct {
		Results []websearch.SearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return nil
	}
	return payload.Results
}

func normalizeGrokSearchURL(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), true
}

func usableGrokSearchTitle(title, rawURL string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == rawURL {
		return ""
	}
	if _, err := strconv.Atoi(title); err == nil {
		return ""
	}
	return title
}

func grokSearchTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}
