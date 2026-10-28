//go:build unit

package service

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var sync20261028BenchmarkResult string

func BenchmarkUpstreamSync20261028(b *testing.B) {
	for _, tc := range []struct { name, text string }{
		{"FingerprintASCII", "abcdefghijklmnopqrstuvwxyz"},
		{"FingerprintLongUnicode", strings.Repeat("界😀", 1<<18)},
	} {
		body, err := json.Marshal(map[string]any{"messages": []map[string]any{{"role":"user","content":tc.text}}})
		if err != nil { b.Fatal(err) }
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() { sync20261028BenchmarkResult = computeClaudeCodeFingerprint(body, "2.1.161") }
		})
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct { name, payload string }{
		{"GeminiText", `{"candidates":[{"content":{"parts":[{"text":"ordinary output"}]}}]}`},
		{"GeminiInline", `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"eA=="}}]}}]}`},
	} {
		payload := []byte(tc.payload)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		beginGeminiImageOutputObservation(c)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() { observeGeminiImageOutputs(c, payload) }
		})
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"messages":[{"role":"developer","content":"`+strings.Repeat("x", 1<<20)+`"}],"model":"deepseek-v4-pro"}`)
	b.Run("UnrelatedLargeChat", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := normalizeStrictChatDeveloperRoles(account, "https://api.openai.com/v1/chat/completions", body); err != nil { b.Fatal(err) }
		}
	})
}
