package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/google/uuid"
)

const clusterNodeIDFileName = ".cluster-node-id"

func resolveClusterNodeID(cfg *config.Config) (string, error) {
	if cfg != nil {
		if explicit := strings.TrimSpace(cfg.Deployment.NodeID); explicit != "" {
			if err := validateClusterNodeID(explicit); err != nil {
				return "", err
			}
			return explicit, nil
		}
	}

	path := ""
	if cfg != nil {
		path = strings.TrimSpace(cfg.Deployment.NodeIDFile)
	}
	if path == "" {
		dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
		if dataDir == "" {
			if info, err := os.Stat("/app/data"); err == nil && info.IsDir() {
				dataDir = "/app/data"
			} else {
				dataDir = "."
			}
		}
		path = filepath.Join(dataDir, clusterNodeIDFileName)
	}

	if data, err := os.ReadFile(path); err == nil {
		nodeID := strings.TrimSpace(string(data))
		if err := validateClusterNodeID(nodeID); err != nil {
			return "", fmt.Errorf("invalid cluster node id file %s: %w", path, err)
		}
		return nodeID, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read cluster node id file %s: %w", path, err)
	}

	nodeID := uuid.NewString()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read concurrently created cluster node id file %s: %w", path, readErr)
		}
		nodeID = strings.TrimSpace(string(data))
		if validateErr := validateClusterNodeID(nodeID); validateErr != nil {
			return "", fmt.Errorf("invalid concurrently created cluster node id file %s: %w", path, validateErr)
		}
		return nodeID, nil
	}
	if err != nil {
		return "", fmt.Errorf("create cluster node id file %s: %w", path, err)
	}
	if _, err := file.WriteString(nodeID + "\n"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write cluster node id file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close cluster node id file %s: %w", path, err)
	}
	return nodeID, nil
}

func validateClusterNodeID(value string) error {
	if value == "" {
		return errors.New("node id is empty")
	}
	if len(value) > 64 {
		return errors.New("node id exceeds 64 characters")
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return fmt.Errorf("node id contains unsupported character %q", char)
	}
	return nil
}
