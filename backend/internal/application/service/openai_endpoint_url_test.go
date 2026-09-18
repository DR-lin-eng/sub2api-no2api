package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIEndpointURLPreservesURLComponents(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		endpoint string
		want     string
	}{
		{name: "root", base: "https://upstream.example", endpoint: "/v1/models", want: "https://upstream.example/v1/models"},
		{name: "v1", base: "https://upstream.example/v1", endpoint: "/v1/responses", want: "https://upstream.example/v1/responses"},
		{name: "prefix", base: "https://upstream.example/openai", endpoint: "/v1/chat/completions", want: "https://upstream.example/openai/v1/chat/completions"},
		{name: "version", base: "https://upstream.example/openai/v2", endpoint: "/v1/embeddings", want: "https://upstream.example/openai/v2/embeddings"},
		{name: "query", base: "https://upstream.example/v1?redirect=/", endpoint: "/v1/sub2api/billing", want: "https://upstream.example/v1/sub2api/billing?redirect=/"},
		{name: "fragment is removed", base: "https://upstream.example/v1#stale", endpoint: "/v1/alpha/search", want: "https://upstream.example/v1/alpha/search"},
		{name: "ipv6", base: "http://[2001:db8::1]:8080/v1?tenant=a#stale", endpoint: "/v1/responses/input_tokens", want: "http://[2001:db8::1]:8080/v1/responses/input_tokens?tenant=a"},
		{name: "already complete", base: "https://upstream.example/v1/images/generations?tenant=a", endpoint: "/v1/images/generations", want: "https://upstream.example/v1/images/generations?tenant=a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, buildOpenAIEndpointURL(tt.base, tt.endpoint))
		})
	}
}

func TestBuildOpenAIOAuthCodexResponsesURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "codex relay path", base: "https://codex-relay.example/backend-api/codex", want: "https://codex-relay.example/backend-api/codex/responses"},
		{name: "already responses", base: "https://codex-relay.example/backend-api/codex/responses", want: "https://codex-relay.example/backend-api/codex/responses"},
		{name: "versioned relay", base: "https://relay.example/v1", want: "https://relay.example/v1/responses"},
		{name: "query preserved", base: "https://relay.example/backend-api/codex?tenant=one", want: "https://relay.example/backend-api/codex/responses?tenant=one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, buildOpenAIOAuthCodexResponsesURL(tt.base))
		})
	}
}
