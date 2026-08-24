package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamClientsUseChatGPTCookieJar(t *testing.T) {
	codexsimulation.SetCLevelEnabled(true)
	t.Cleanup(func() { codexsimulation.SetCLevelEnabled(false) })
	upstream, ok := NewHTTPUpstream(nil, nil).(*httpUpstreamService)
	require.True(t, ok)
	entry, err := upstream.getClientEntryRoute(
		platformegress.DirectRoute(false),
		101,
		1,
		service.HTTPUpstreamProfileOpenAI,
		false,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, entry.client.Jar)
}
