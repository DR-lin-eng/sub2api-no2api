package repository

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

const (
	heControlDesiredFile = "desired.env"
	heControlRequestFile = "request.env"
	heControlStatusFile  = "status.env"
)

type fileHETunnelControlStore struct {
	dir          string
	staleAfter   time.Duration
	mu           sync.Mutex
	randomReader func([]byte) (int, error)
}

func NewHETunnelControlStore(cfg *config.Config) moduleegress.HETunnelControlStore {
	dir := "/app/data/ipv6-egress-control"
	staleAfter := 15 * time.Second
	if cfg != nil {
		if configured := strings.TrimSpace(cfg.IPv6Egress.ControlDir); configured != "" {
			dir = configured
		}
		if cfg.IPv6Egress.ControlAgentStaleSeconds > 0 {
			staleAfter = time.Duration(cfg.IPv6Egress.ControlAgentStaleSeconds) * time.Second
		}
	}
	return &fileHETunnelControlStore{
		dir:          dir,
		staleAfter:   staleAfter,
		randomReader: rand.Read,
	}
}

func (s *fileHETunnelControlStore) Load(ctx context.Context) (*moduleegress.HETunnelControlSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create HE tunnel control directory: %w", err)
	}

	configValues, err := readHETunnelControlEnv(filepath.Join(s.dir, heControlDesiredFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read HE tunnel configuration: %w", err)
	}
	requestValues, err := readHETunnelControlEnv(filepath.Join(s.dir, heControlRequestFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read HE tunnel request: %w", err)
	}
	statusValues, err := readHETunnelControlEnv(filepath.Join(s.dir, heControlStatusFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read HE tunnel agent status: %w", err)
	}

	snapshot := &moduleegress.HETunnelControlSnapshot{
		Config: decodeHETunnelConfig(configValues),
		Agent:  decodeHETunnelAgentStatus(statusValues, s.staleAfter),
	}
	requestID := requestValues["IPV6_EGRESS_REQUEST_ID"]
	if requestID != "" && requestID != snapshot.Agent.RequestID {
		snapshot.Agent.State = "pending"
		snapshot.Agent.Action = requestValues["IPV6_EGRESS_REQUEST_ACTION"]
		snapshot.Agent.RequestID = requestID
		snapshot.Agent.Message = ""
	}
	return snapshot, nil
}

func (s *fileHETunnelControlStore) SaveConfig(ctx context.Context, value moduleegress.HETunnelConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create HE tunnel control directory: %w", err)
	}
	lines := []string{
		"HE_TUNNEL_ENABLED=" + strconv.FormatBool(value.Enabled),
		"HE_TUNNEL_SERVER_IPV4=" + value.ServerIPv4,
		"HE_TUNNEL_LOCAL_IPV4=" + value.LocalIPv4,
		"HE_TUNNEL_CLIENT_IPV6=" + value.ClientIPv6,
		"HE_TUNNEL_SERVER_IPV6=" + value.ServerIPv6,
		"IPV6_EGRESS_POOL_CIDR=" + value.PoolCIDR,
		"HE_TUNNEL_MTU=" + strconv.Itoa(value.MTU),
		"HE_TUNNEL_TTL=255",
		"HE_TUNNEL_ROUTE_METRIC=" + strconv.Itoa(value.RouteMetric),
		"HE_TUNNEL_PROBE_IPV6=" + value.ProbeIPv6,
		"HE_TUNNEL_PROBE_TIMEOUT_SECONDS=" + strconv.Itoa(value.ProbeTimeoutSeconds),
		"HE_TUNNEL_ALLOW_PRIVATE_IPV4=" + strconv.FormatBool(value.AllowPrivateIPv4),
		"HE_TUNNEL_UPDATE_ENABLED=" + strconv.FormatBool(value.UpdateEnabled),
		"HE_TUNNEL_ID=" + value.TunnelID,
		"HE_TUNNEL_USERNAME=" + value.Username,
		"HE_TUNNEL_UPDATE_KEY=" + value.UpdateKey,
	}
	return writeHETunnelControlFile(filepath.Join(s.dir, heControlDesiredFile), strings.Join(lines, "\n")+"\n", 0o600)
}

func (s *fileHETunnelControlStore) Request(ctx context.Context, action string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("create HE tunnel control directory: %w", err)
	}
	random := make([]byte, 16)
	if _, err := s.randomReader(random); err != nil {
		return "", fmt.Errorf("generate HE tunnel request ID: %w", err)
	}
	requestID := hex.EncodeToString(random)
	body := strings.Join([]string{
		"IPV6_EGRESS_REQUEST_ID=" + requestID,
		"IPV6_EGRESS_REQUEST_ACTION=" + action,
		"IPV6_EGRESS_REQUESTED_AT_UNIX=" + strconv.FormatInt(time.Now().Unix(), 10),
	}, "\n") + "\n"
	if err := writeHETunnelControlFile(filepath.Join(s.dir, heControlRequestFile), body, 0o600); err != nil {
		return "", err
	}
	return requestID, nil
}

func readHETunnelControlEnv(path string) (values map[string]string, retErr error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close HE tunnel control file: %w", err)
		}
	}()

	values = make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func writeHETunnelControlFile(path, content string, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".he-control-*")
	if err != nil {
		return fmt.Errorf("create temporary HE tunnel control file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	closeWithError := func(cause error) error {
		if closeErr := tmp.Close(); closeErr != nil {
			return errors.Join(cause, fmt.Errorf("close temporary HE tunnel control file: %w", closeErr))
		}
		return cause
	}
	if err := tmp.Chmod(mode); err != nil {
		return closeWithError(err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		return closeWithError(err)
	}
	if err := tmp.Sync(); err != nil {
		return closeWithError(err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish HE tunnel control file: %w", err)
	}
	return nil
}

func decodeHETunnelConfig(values map[string]string) moduleegress.HETunnelConfig {
	allowPrivateIPv4 := true
	if raw, ok := values["HE_TUNNEL_ALLOW_PRIVATE_IPV4"]; ok {
		allowPrivateIPv4 = parseHETunnelBool(raw)
	}
	return moduleegress.HETunnelConfig{
		Enabled:             parseHETunnelBool(values["HE_TUNNEL_ENABLED"]),
		ServerIPv4:          values["HE_TUNNEL_SERVER_IPV4"],
		LocalIPv4:           values["HE_TUNNEL_LOCAL_IPV4"],
		ClientIPv6:          values["HE_TUNNEL_CLIENT_IPV6"],
		ServerIPv6:          values["HE_TUNNEL_SERVER_IPV6"],
		PoolCIDR:            values["IPV6_EGRESS_POOL_CIDR"],
		MTU:                 parseHETunnelInt(values["HE_TUNNEL_MTU"]),
		RouteMetric:         parseHETunnelInt(values["HE_TUNNEL_ROUTE_METRIC"]),
		ProbeIPv6:           values["HE_TUNNEL_PROBE_IPV6"],
		ProbeTimeoutSeconds: parseHETunnelInt(values["HE_TUNNEL_PROBE_TIMEOUT_SECONDS"]),
		AllowPrivateIPv4:    allowPrivateIPv4,
		UpdateEnabled:       parseHETunnelBool(values["HE_TUNNEL_UPDATE_ENABLED"]),
		TunnelID:            values["HE_TUNNEL_ID"],
		Username:            values["HE_TUNNEL_USERNAME"],
		UpdateKey:           values["HE_TUNNEL_UPDATE_KEY"],
	}
}

func decodeHETunnelAgentStatus(values map[string]string, staleAfter time.Duration) moduleegress.HETunnelAgentStatus {
	heartbeatUnix, _ := strconv.ParseInt(values["IPV6_EGRESS_STATUS_HEARTBEAT_UNIX"], 10, 64)
	updatedUnix, _ := strconv.ParseInt(values["IPV6_EGRESS_STATUS_UPDATED_AT_UNIX"], 10, 64)
	status := moduleegress.HETunnelAgentStatus{
		State:     values["IPV6_EGRESS_STATUS_STATE"],
		Action:    values["IPV6_EGRESS_STATUS_ACTION"],
		Message:   values["IPV6_EGRESS_STATUS_MESSAGE"],
		RequestID: values["IPV6_EGRESS_STATUS_REQUEST_ID"],
	}
	if status.State == "" {
		status.State = "offline"
	}
	if heartbeatUnix > 0 {
		heartbeat := time.Unix(heartbeatUnix, 0)
		age := time.Since(heartbeat)
		status.Online = age >= 0 && age <= staleAfter
	}
	if updatedUnix > 0 {
		updated := time.Unix(updatedUnix, 0).UTC()
		status.UpdatedAt = &updated
	}
	return status
}

func parseHETunnelBool(raw string) bool {
	parsed, _ := strconv.ParseBool(strings.TrimSpace(raw))
	return parsed
}

func parseHETunnelInt(raw string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(raw))
	return parsed
}
