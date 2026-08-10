package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestResolveClusterNodeIDPersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node-id")
	cfg := &config.Config{Deployment: config.DeploymentConfig{NodeIDFile: path}}

	first, err := resolveClusterNodeID(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := resolveClusterNodeID(cfg)
	require.NoError(t, err)
	require.Equal(t, first, second)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, first+"\n", string(data))
}

func TestResolveClusterNodeIDUsesValidatedExplicitIdentity(t *testing.T) {
	cfg := &config.Config{Deployment: config.DeploymentConfig{NodeID: "host-cn-01"}}
	nodeID, err := resolveClusterNodeID(cfg)
	require.NoError(t, err)
	require.Equal(t, "host-cn-01", nodeID)

	cfg.Deployment.NodeID = "invalid node id"
	_, err = resolveClusterNodeID(cfg)
	require.Error(t, err)
}

func TestResolveClusterNodeIDRejectsCorruptIdentityFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node-id")
	require.NoError(t, os.WriteFile(path, []byte("bad node id\n"), 0o600))

	_, err := resolveClusterNodeID(&config.Config{Deployment: config.DeploymentConfig{NodeIDFile: path}})
	require.Error(t, err)
}
