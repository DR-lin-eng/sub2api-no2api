package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeProxyProbeURLs(t *testing.T) {
	got, err := normalizeProxyProbeURLs([]ProbeURLConfig{
		{URL: " https://chatgpt.com/cdn-cgi/trace ", Parser: " CHATGPT-TRACE "},
		{URL: "https://api64.ipify.org?format=json", Parser: "ipify"},
	})
	require.NoError(t, err)
	require.Equal(t, []ProbeURLConfig{
		{URL: "https://chatgpt.com/cdn-cgi/trace", Parser: "chatgpt-trace"},
		{URL: "https://api64.ipify.org?format=json", Parser: "ipify"},
	}, got)
}

func TestNormalizeProxyProbeURLsRejectsInvalidEntries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target ProbeURLConfig
		want   string
	}{
		{name: "missing URL", target: ProbeURLConfig{Parser: "ipify"}, want: "url is required"},
		{name: "missing parser", target: ProbeURLConfig{URL: "https://example.com"}, want: "parser is required"},
		{name: "unknown parser", target: ProbeURLConfig{URL: "https://example.com", Parser: "ip_api"}, want: "unsupported parser"},
		{name: "relative URL", target: ProbeURLConfig{URL: "/trace", Parser: "chatgpt-trace"}, want: "invalid url"},
		{name: "unsupported scheme", target: ProbeURLConfig{URL: "ftp://example.com", Parser: "ipify"}, want: "scheme must be http or https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeProxyProbeURLs([]ProbeURLConfig{tc.target})
			require.ErrorContains(t, err, tc.want)
		})
	}
}
