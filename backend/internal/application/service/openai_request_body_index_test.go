package service

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestOpenAIRequestBodyIndexMatchesGJSON(t *testing.T) {
	bodies := []string{
		``,
		`{}`,
		`[]`,
		`[{"model":"in-array"}]`,
		`"model"`,
		`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hi"}],"stream":true,"reasoning":{"effort":"high","summary":null},"store":false}`,
		`{"model":" spaced ","model":"second","reasoning":"scalar","reasoning":{"effort":"low"},"reasoning":{"effort":"later"}}`,
		`{"reasoning":{"other":1},"reasoning":{"effort":"second-object"}}`,
		`{"mo\u0064el":"escaped-key","input":"text \"quoted\" \u00e9","max_output_tokens":128,"metadata":{"user_id":"u.1","a.b":2},"tools":[{"type":"function","name":"f"},{"type":"web_search"}]}`,
		` garbage {"model":"prefixed","stream":false}`,
		`{"model":"truncated","input":[1,2`,
		`{"model":"m"} {"stream":true}`,
		`{"a":{"model":"nested"}},"model":"after-root"`,
		`{"input":null,"previous_response_id":"","service_tier":0,"flag":true}`,
	}
	paths := []string{
		"model", "stream", "store", "input", "tools", "reasoning", "reasoning.effort", "reasoning.summary",
		"reasoning.missing", "previous_response_id", "service_tier", "max_output_tokens", "metadata.user_id",
		`metadata.a\.b`, "tools.0.type", "tools.#", "tools.1", "input.0.role", `input.#(type=="message")`,
		"missing", "flag", "mo*", "model|@pretty", "@this", "", "a.model", "tools.#.type",
	}
	for _, body := range bodies {
		index := newOpenAIRequestBodyIndex([]byte(body))
		for _, path := range paths {
			want := gjson.GetBytes([]byte(body), path)
			for name, got := range map[string]gjson.Result{"Lookup": index.Lookup(path), "Get": index.Get(path)} {
				require.Equalf(t, want.Exists(), got.Exists(), "%s exists body=%s path=%q", name, body, path)
				require.Equalf(t, want.Type, got.Type, "%s type body=%s path=%q", name, body, path)
				require.Equalf(t, want.Raw, got.Raw, "%s raw body=%s path=%q", name, body, path)
				require.Equalf(t, want.String(), got.String(), "%s string body=%s path=%q", name, body, path)
				require.Equalf(t, want.Num, got.Num, "%s num body=%s path=%q", name, body, path)
			}
		}
	}
}

func TestOpenAIRequestBodyIndexInputItemTypes(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"x"},{"type":"function_call_output","output":"y"},"plain",{"type":"message"},{"type":"function_call_output"},{"type":7}],"model":"m"}`)
	index := newOpenAIRequestBodyIndex(body)
	require.Equal(t, []string{"", "function_call_output", "message", "7"}, index.InputItemTypes())
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, OpenAIRequestBodyMayHaveInputItemType(c, body, "function_call_output"))
	require.False(t, OpenAIRequestBodyMayHaveInputItemType(c, body, "compaction_trigger"))
	// Without a request context, and for bodies whose readers can disagree,
	// the pre-check never rules an item type out.
	require.True(t, OpenAIRequestBodyMayHaveInputItemType(nil, body, "compaction_trigger"))
	for _, ambiguous := range []string{
		`x {"input":[{"type":"message"}]}`,
		`{"input":[{"type":"message"}],"input":[{"type":"compaction_trigger"}]}`,
		`[{"type":"message"}]`,
	} {
		require.True(t, OpenAIRequestBodyMayHaveInputItemType(c, []byte(ambiguous), "compaction_trigger"), ambiguous)
	}

	require.Empty(t, newOpenAIRequestBodyIndex([]byte(`{"input":"text"}`)).InputItemTypes())
	require.Empty(t, newOpenAIRequestBodyIndex([]byte(`{"model":"m"}`)).InputItemTypes())
}

func TestOpenAIRequestBodyIndexInputItemTypesManyDistinct(t *testing.T) {
	const distinct = 200000
	var body strings.Builder
	want := make([]string, 0, distinct)
	_, _ = body.WriteString(`{"input":[`)
	for i := 0; i < distinct; i++ {
		itemType := "t" + strconv.Itoa(i)
		want = append(want, itemType)
		if i > 0 {
			_ = body.WriteByte(',')
		}
		// Every type appears twice so the duplicate check runs on both sides
		// of the switch to the set.
		_, _ = body.WriteString(`{"type":"` + itemType + `"},{"type":"` + itemType + `"}`)
	}
	_, _ = body.WriteString(`]}`)

	start := time.Now()
	got := newOpenAIRequestBodyIndex([]byte(body.String())).InputItemTypes()
	elapsed := time.Since(start)

	require.Equal(t, want, got)
	// The quadratic duplicate check needed about 2e10 comparisons here.
	require.Less(t, elapsed, 5*time.Second)
}

func TestOpenAIRequestBodyIndexForFollowsBodyRewrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"input":[{"role":"user","content":"hello"}],"model":"gpt-5.5","stream":true}`)

	first := openAIRequestBodyIndexFor(c, body)
	require.Same(t, first, openAIRequestBodyIndexFor(c, body))
	require.Equal(t, "gpt-5.5", OpenAIRequestBodyField(c, body, "model").String())

	rewritten, err := sjson.SetBytes(body, "model", "gpt-5.5-codex")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5-codex", OpenAIRequestBodyField(c, rewritten, "model").String())
	require.NotSame(t, first, openAIRequestBodyIndexFor(c, rewritten))
	// The superseded body is still answered correctly, from a fresh index.
	require.Equal(t, "gpt-5.5", OpenAIRequestBodyField(c, body, "model").String())

	// Same content in a different slice is a different body.
	clone := append([]byte(nil), body...)
	require.False(t, first.covers(clone))
	require.True(t, first.covers(body))
	require.False(t, first.covers(body[:len(body)-1]))

	stored := OpenAIRequestBodyField(c, body, "model")
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "gpt-5.5", stored.String(), "Get must detach scalars from the body")
}

func newOpenAIRequestBodyIndexTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}
