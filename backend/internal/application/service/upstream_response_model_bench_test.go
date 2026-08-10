package service

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

var upstreamResponseModelBenchmarkSink string

func BenchmarkUpstreamResponseModelOpenAI(b *testing.B) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "terminal_with_model", payload: []byte(`{"type":"response.completed","response":{"model":"gpt-5.5-2026-04-23"}}`)},
		{name: "delta_without_model", payload: []byte(`{"type":"response.output_text.delta","delta":"hello"}`)},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.Run("legacy", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					upstreamResponseModelBenchmarkSink = benchmarkLegacyOpenAIModel(tt.payload)
				}
			})
			b.Run("optimized", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					upstreamResponseModelBenchmarkSink = firstValidTrimmedGJSONModel(tt.payload, "response.model", "model")
				}
			})
		})
	}
}

func BenchmarkUpstreamResponseModelAntigravityWrapper(b *testing.B) {
	payload := []byte(`{"response":{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}}`)
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			inner := gjson.GetBytes(payload, "response")
			upstreamResponseModelBenchmarkSink = benchmarkLegacyGeminiModel([]byte(inner.Raw))
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			upstreamResponseModelBenchmarkSink = firstValidTrimmedGJSONModel(payload,
				"modelVersion", "response.modelVersion", "response.response.modelVersion")
		}
	})
}

func benchmarkLegacyOpenAIModel(payload []byte) string {
	if !gjson.ValidBytes(payload) {
		return ""
	}
	return benchmarkFirstTrimmedGJSONModel(gjson.GetBytes(payload, "response.model"), gjson.GetBytes(payload, "model"))
}

func benchmarkLegacyGeminiModel(payload []byte) string {
	if !gjson.ValidBytes(payload) {
		return ""
	}
	return benchmarkFirstTrimmedGJSONModel(gjson.GetBytes(payload, "modelVersion"), gjson.GetBytes(payload, "response.modelVersion"))
}

func benchmarkFirstTrimmedGJSONModel(values ...gjson.Result) string {
	for _, value := range values {
		if value.Exists() && value.Type == gjson.String {
			if model := strings.TrimSpace(value.String()); model != "" {
				return model
			}
		}
	}
	return ""
}
