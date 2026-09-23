package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorEndpointPathPrefixIsPreserved(t *testing.T) {
	require.Equal(t, "https://provider.example/anthropic/v1/messages", joinURL("https://provider.example/anthropic", "/v1/messages"))
	require.Equal(t, "https://provider.example/v1/messages", joinURL("https://provider.example/v1", "/v1/messages"))
	require.Equal(t, "https://provider.example/root/v1/messages", joinURL("https://provider.example/root", "/v1/messages"))
}
