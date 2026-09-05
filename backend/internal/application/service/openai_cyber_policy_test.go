package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkAndGetOpsCyberPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	require.Nil(t, GetOpsCyberPolicy(c), "no mark initially")

	MarkOpsCyberPolicy(c, CyberPolicyMark{
		Code:           "cyber_policy",
		Message:        "This request was flagged for cyber policy.",
		Body:           `{"error":{"code":"cyber_policy"}}`,
		UpstreamStatus: 400,
	})

	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got)
	require.Equal(t, "cyber_policy", got.Code)
	require.Equal(t, 400, got.UpstreamStatus)
}

func TestMarkOpsCyberPolicyFirstWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "first"})
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "second"})
	require.Equal(t, "first", GetOpsCyberPolicy(c).Message, "first mark wins, later marks ignored")
}

func TestMarkOpsCyberPolicyNilContext(t *testing.T) {
	MarkOpsCyberPolicy(nil, CyberPolicyMark{Code: "cyber_policy"})
	require.Nil(t, GetOpsCyberPolicy(nil))
}

// TestClearOpsCyberPolicy_AllowsRemark verifies F1: after Clear, Get returns nil
// and a subsequent Mark takes effect (per-turn lifecycle in WS connections).
func TestClearOpsCyberPolicy_AllowsRemark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "first", UpstreamStatus: 200})
	require.NotNil(t, GetOpsCyberPolicy(c))

	ClearOpsCyberPolicy(c)
	require.Nil(t, GetOpsCyberPolicy(c), "mark must be invisible after Clear")

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "second", UpstreamStatus: 400})
	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got, "re-mark after Clear must take effect")
	require.Equal(t, "second", got.Message)
}

func TestDetectOpenAICyberPolicy(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		hit     bool
		msg     string
	}{
		{"top-level error", `{"error":{"code":"cyber_policy","message":"flagged"}}`, true, "flagged"},
		{"response-wrapped", `{"response":{"error":{"code":"cyber_policy","message":"  bad  "}}}`, true, "bad"},
		{"case-insensitive", `{"error":{"code":"Cyber_Policy"}}`, true, ""},
		{"content_policy not cyber", `{"error":{"code":"content_policy","message":"x"}}`, false, ""},
		{"safety message not cyber", `{"error":{"type":"safety_error","message":"high-risk cyber activity"}}`, false, ""},
		{"empty", ``, false, ""},
		{"upstream_error", `{"error":{"code":"upstream_error"}}`, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit, code, msg := detectOpenAICyberPolicy([]byte(tc.payload))
			require.Equal(t, tc.hit, hit)
			if tc.hit {
				require.Equal(t, "cyber_policy", code)
				require.Equal(t, tc.msg, msg)
			}
		})
	}
}

func TestMarkOpenAICyberPolicyEventBounded(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	payload := []byte(`{"error":{"code":"cyber_policy","message":"blocked"},"padding":"` + strings.Repeat("x", 100000) + `"}`)
	require.True(t, markOpenAICyberPolicyEvent(c, payload, 403, &OpenAIUsage{InputTokens: 3, OutputTokens: 5}))
	mark := GetOpsCyberPolicy(c)
	require.Len(t, mark.Body, 4096)
	require.Equal(t, 403, mark.UpstreamStatus)
	require.EqualValues(t, 3, mark.UpstreamInTok)
	require.EqualValues(t, 5, mark.UpstreamOutTok)
	require.False(t, markOpenAICyberPolicyEvent(c, []byte(`{"error":{"code":"rate_limit_exceeded"}}`), 429, nil))
	require.Equal(t, mark, GetOpsCyberPolicy(c))
}

func TestMarkOpenAICyberPolicyEventUsageShapes(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		input, output int
	}{
		{"bare_error", `{"type":"error","error":{"code":"cyber_policy","message":"blocked"},"usage":{"input_tokens":5,"output_tokens":1}}`, 5, 1},
		{"response_failed", `{"type":"response.failed","response":{"error":{"code":"cyber_policy"},"usage":{"input_tokens":9,"output_tokens":2}}}`, 9, 2},
		{"prior_usage", `{"type":"error","error":{"code":"cyber_policy"}}`, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			prior := OpenAIUsage{InputTokens: 3, OutputTokens: 4}
			require.True(t, markOpenAICyberPolicyEvent(c, []byte(tc.payload), 200, &prior))
			require.Equal(t, tc.input, GetOpsCyberPolicy(c).UpstreamInTok)
			require.Equal(t, tc.output, GetOpsCyberPolicy(c).UpstreamOutTok)
			require.Equal(t, 3, prior.InputTokens, "recording must not mutate caller usage")
		})
	}
}
