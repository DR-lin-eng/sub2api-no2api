package tlsfingerprint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultNetworkDialerHasBoundedTimeout(t *testing.T) {
	require.Equal(t, defaultDialTimeout, defaultNetworkDialer.Timeout)
	require.Equal(t, defaultDialKeepAlive, defaultNetworkDialer.KeepAlive)
	require.Greater(t, defaultNetworkDialer.Timeout, time.Duration(0))
	require.Greater(t, defaultTLSHandshakeTimeout, time.Duration(0))
}
