//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesCreatedAtStamperAddsStableTimestampOnlyWhenMissing(t *testing.T) {
	stamper := responsesCreatedAtStamper{}
	first, changed := stamper.Stamp([]byte(`{"type":"response.created","response":{"id":"r1"}}`), "response.created")
	require.True(t, changed)
	firstAt := responseCreatedAtForTest(t, first)
	require.Greater(t, firstAt, int64(0))

	second, changed := stamper.Stamp([]byte(`{"type":"response.completed","response":{"id":"r1"}}`), "response.completed")
	require.True(t, changed)
	require.Equal(t, firstAt, responseCreatedAtForTest(t, second))

	existing := []byte(`{"type":"response.completed","response":{"created_at":1700000000,"id":"r1"}}`)
	unchanged, changed := stamper.Stamp(existing, "response.completed")
	require.False(t, changed)
	require.Equal(t, string(existing), string(unchanged))
}

func TestResponsesCreatedAtStamperLearnsUpstreamValue(t *testing.T) {
	stamper := responsesCreatedAtStamper{}
	stamper.Observe([]byte(`{"type":"response.created","response":{"created_at":1700000123}}`), "response.created")
	updated, changed := stamper.Stamp([]byte(`{"type":"response.failed","response":{"status":"failed"}}`), "response.failed")
	require.True(t, changed)
	require.Equal(t, int64(1700000123), responseCreatedAtForTest(t, updated))
}

func TestEnsureOpenAIResponseObjectCreatedAtPreservesNonResponses(t *testing.T) {
	response := ensureOpenAIResponseObjectCreatedAt([]byte(`{"object":"response","id":"r1"}`))
	require.Greater(t, responseCreatedAtForTest(t, response), int64(0))

	errorBody := []byte(`{"error":{"message":"failed"}}`)
	require.Equal(t, string(errorBody), string(ensureOpenAIResponseObjectCreatedAt(errorBody)))

	existing := []byte(`{"object":"response","created_at":0,"id":"r1"}`)
	require.Equal(t, string(existing), string(ensureOpenAIResponseObjectCreatedAt(existing)))
}

func responseCreatedAtForTest(t *testing.T, payload []byte) int64 {
	t.Helper()
	var value struct {
		Response struct {
			CreatedAt int64 `json:"created_at"`
		} `json:"response"`
		CreatedAt int64 `json:"created_at"`
	}
	require.NoError(t, json.Unmarshal(payload, &value))
	if value.Response.CreatedAt != 0 {
		return value.Response.CreatedAt
	}
	return value.CreatedAt
}

func BenchmarkResponsesCreatedAtStamperExisting(b *testing.B) {
	payload := []byte(`{"type":"response.completed","response":{"created_at":1700000000,"output":[]}}`)
	stamper := responsesCreatedAtStamper{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stamper.Stamp(payload, "response.completed")
	}
}

func BenchmarkResponsesCreatedAtStamperMissing(b *testing.B) {
	payload := []byte(`{"type":"response.completed","response":{"output":[]}}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stamper := responsesCreatedAtStamper{}
		stamper.Stamp(payload, "response.completed")
	}
}
