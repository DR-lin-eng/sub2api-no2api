//go:build unit

package handler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildGrokXSearchResponsesBody(t *testing.T) {
	t.Parallel()
	images := true
	videos := false
	body, err := buildGrokXSearchResponsesBody(grokXSearchRequest{
		Query:                    "latest posts from xAI",
		AllowedXHandles:          []string{"xai"},
		ExcludedXHandles:         []string{"spam"},
		FromDate:                 " 2026-08-01 ",
		ToDate:                   "2026-08-10",
		EnableImageUnderstanding: &images,
		EnableVideoUnderstanding: &videos,
	}, "grok-4.6-latest", 50)
	require.NoError(t, err)
	require.Equal(t, "grok-4.6", gjson.GetBytes(body, "model").String())
	require.Contains(t, gjson.GetBytes(body, "input").String(), "latest posts from xAI")
	require.Contains(t, gjson.GetBytes(body, "input").String(), "at most 20 unique results")
	require.Equal(t, "required", gjson.GetBytes(body, "tool_choice").String())
	require.Equal(t, "x_search_call.action.sources", gjson.GetBytes(body, "include.0").String())
	require.Equal(t, "x_search", gjson.GetBytes(body, "tools.0.type").String())
	require.Equal(t, "xai", gjson.GetBytes(body, "tools.0.allowed_x_handles.0").String())
	require.Equal(t, "spam", gjson.GetBytes(body, "tools.0.excluded_x_handles.0").String())
	require.Equal(t, "2026-08-01", gjson.GetBytes(body, "tools.0.from_date").String())
	require.Equal(t, "2026-08-10", gjson.GetBytes(body, "tools.0.to_date").String())
	require.True(t, gjson.GetBytes(body, "tools.0.enable_image_understanding").Bool())
	require.False(t, gjson.GetBytes(body, "tools.0.enable_video_understanding").Bool())
	require.False(t, gjson.GetBytes(body, "store").Bool())
	require.False(t, gjson.GetBytes(body, "stream").Bool())
}

func TestExtractGrokSearchSourcesAdmitsOnlyObservedSources(t *testing.T) {
	t.Parallel()
	body := []byte(`{
  "output": [
    {"type":"x_search_call","action":{"sources":[
      {"url":"https://x.com/xai/status/1","title":"Source title","snippet":"source summary"},
      {"url":"https://example.com/post","title":"Example"}
    ]}},
    {"type":"message","content":[{"type":"output_text","text":"{\"results\":[{\"url\":\"https://x.com/xai/status/1#fragment\",\"title\":\"Model title\",\"snippet\":\"model summary\"},{\"url\":\"https://invented.invalid/post\",\"title\":\"Invented\",\"snippet\":\"must be rejected\"},{\"url\":\"https://example.com/post\",\"title\":\"Example enriched\",\"snippet\":\"enriched\"}]}"}]}
  ]
}`)

	results := extractGrokSearchSources(body, 20)
	require.Len(t, results, 2)
	require.Equal(t, "https://x.com/xai/status/1", results[0].URL)
	require.Equal(t, "Model title", results[0].Title)
	require.Equal(t, "model summary", results[0].Snippet)
	require.Equal(t, "https://example.com/post", results[1].URL)
	require.Equal(t, "Example enriched", results[1].Title)
	require.Equal(t, "enriched", results[1].Snippet)
}

func TestExtractGrokSearchSourcesDeduplicatesAndCapsAtTwenty(t *testing.T) {
	t.Parallel()
	body := `{"output":[{"type":"x_search_call","action":{"sources":[`
	for i := 0; i < 25; i++ {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"url":"https://x.com/user/status/%d","title":"Post %d"}`, i, i)
	}
	body += `,{"url":"https://x.com/user/status/0#duplicate","title":"Duplicate"}]}}]}`

	results := extractGrokSearchSources([]byte(body), 99)
	require.Len(t, results, 20)
	require.Equal(t, "https://x.com/user/status/0", results[0].URL)
	require.Equal(t, "https://x.com/user/status/19", results[19].URL)
}

func TestNormalizeGrokXSearchMaxResults(t *testing.T) {
	t.Parallel()
	require.Equal(t, defaultGrokXSearchResults, normalizeGrokXSearchMaxResults(0))
	require.Equal(t, defaultGrokXSearchResults, normalizeGrokXSearchMaxResults(-1))
	require.Equal(t, 7, normalizeGrokXSearchMaxResults(7))
	require.Equal(t, maxGrokXSearchResults, normalizeGrokXSearchMaxResults(21))
}
