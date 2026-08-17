package repository

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestFileHETunnelControlRoundTripAndPendingStatus(t *testing.T) {
	dir := t.TempDir()
	store := NewHETunnelControlStore(&config.Config{IPv6Egress: config.IPv6EgressConfig{
		ControlDir:               dir,
		ControlAgentStaleSeconds: 15,
	}})
	value := moduleegress.HETunnelConfig{
		Enabled:             true,
		ServerIPv4:          "216.66.80.30",
		ClientIPv6:          "2001:470:1::2/64",
		ServerIPv6:          "2001:470:1::1",
		PoolCIDR:            "2001:470:2::/64",
		MTU:                 1480,
		RouteMetric:         2048,
		ProbeIPv6:           "2606:4700:4700::1111",
		ProbeTimeoutSeconds: 5,
		AllowPrivateIPv4:    true,
		UpdateEnabled:       true,
		TunnelID:            "12345",
		Username:            "operator",
		UpdateKey:           "secret-update-key",
	}
	require.NoError(t, store.SaveConfig(t.Context(), value))

	snapshot, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, value, snapshot.Config)

	requestID, err := store.Request(t.Context(), moduleegress.HETunnelActionApply)
	require.NoError(t, err)
	require.Len(t, requestID, 32)
	snapshot, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, "pending", snapshot.Agent.State)
	require.Equal(t, requestID, snapshot.Agent.RequestID)
	require.Equal(t, moduleegress.HETunnelActionApply, snapshot.Agent.Action)
}

func TestFileHETunnelControlReadsOnlineAgentStatus(t *testing.T) {
	dir := t.TempDir()
	store := NewHETunnelControlStore(&config.Config{IPv6Egress: config.IPv6EgressConfig{
		ControlDir:               dir,
		ControlAgentStaleSeconds: 15,
	}})
	requestID, err := store.Request(t.Context(), moduleegress.HETunnelActionCheck)
	require.NoError(t, err)
	now := time.Now().Unix()
	status := strings.Join([]string{
		"IPV6_EGRESS_STATUS_REQUEST_ID=" + requestID,
		"IPV6_EGRESS_STATUS_STATE=succeeded",
		"IPV6_EGRESS_STATUS_ACTION=check",
		"IPV6_EGRESS_STATUS_MESSAGE=ready",
		"IPV6_EGRESS_STATUS_UPDATED_AT_UNIX=" + strconv.FormatInt(now, 10),
		"IPV6_EGRESS_STATUS_HEARTBEAT_UNIX=" + strconv.FormatInt(now, 10),
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, heControlStatusFile), []byte(status), 0o644))

	snapshot, err := store.Load(t.Context())
	require.NoError(t, err)
	require.True(t, snapshot.Agent.Online)
	require.Equal(t, "succeeded", snapshot.Agent.State)
	require.Equal(t, "ready", snapshot.Agent.Message)
}
