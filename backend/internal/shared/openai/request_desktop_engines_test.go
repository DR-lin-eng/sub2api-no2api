package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexDesktopEngineIdentities(t *testing.T) {
	for _, name := range []string{"codex_rmcp_cli", "codex_http_cli", "codex_app_server_cli"} {
		t.Run(name, func(t *testing.T) {
			ua := name + "/0.161.0 (Linux 6.8; x86_64) unknown"
			require.True(t, IsCodexOfficialClientRequestStrict(ua))
			require.True(t, IsCodexOfficialClientOriginator(name))
			originator, paired, ok := PairCodexClientIdentity(ua)
			require.True(t, ok)
			require.Equal(t, name, originator)
			require.Equal(t, ua, paired)
			require.False(t, IsCodexOfficialClientRequestStrict("Mozilla/5.0 "+ua))
			require.False(t, IsCodexOfficialClientOriginator("untrusted_"+name))
			_, _, ok = PairCodexClientIdentity(ua + "\r\nInjected: value")
			require.False(t, ok)
		})
	}
}
