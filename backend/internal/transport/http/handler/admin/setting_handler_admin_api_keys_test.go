package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminAPIKeyUpdateExpiryPresence(t *testing.T) {
	var absent adminAPIKeyUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"unchanged expiry"}`), &absent))
	require.False(t, absent.ExpiresAt.Present)

	var cleared adminAPIKeyUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":null}`), &cleared))
	require.True(t, cleared.ExpiresAt.Present)
	require.Nil(t, cleared.ExpiresAt.Value)

	var updated adminAPIKeyUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":"2027-01-31T00:00:00Z"}`), &updated))
	require.True(t, updated.ExpiresAt.Present)
	require.NotNil(t, updated.ExpiresAt.Value)
}
