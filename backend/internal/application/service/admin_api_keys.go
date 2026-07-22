package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Admin API keys are intentionally scoped. A key is shown only once at creation
// or rotation time; the database stores only a SHA-256 digest.
const adminAPIKeysSetting = "admin_api_keys"

const (
	AdminAPIKeyScopeRead          = "admin.read"
	AdminAPIKeyScopeWrite         = "admin.write"
	AdminAPIKeyScopeUsersRead     = "admin.users.read"
	AdminAPIKeyScopeUsersWrite    = "admin.users.write"
	AdminAPIKeyScopeAccountsRead  = "admin.accounts.read"
	AdminAPIKeyScopeAccountsWrite = "admin.accounts.write"
	AdminAPIKeyScopeSettingsRead  = "admin.settings.read"
	AdminAPIKeyScopeSettingsWrite = "admin.settings.write"
	AdminAPIKeyScopeBackupsRead   = "admin.backups.read"
	AdminAPIKeyScopeBackupsWrite  = "admin.backups.write"
	AdminAPIKeyScopeSystemRead    = "admin.system.read"
	AdminAPIKeyScopeSystemWrite   = "admin.system.write"
	AdminAPIKeyScopeAuditRead     = "admin.audit.read"
	AdminAPIKeyScopeAuditWrite    = "admin.audit.write"
	AdminAPIKeyScopeOpsRead       = "admin.ops.read"
	AdminAPIKeyScopeOpsWrite      = "admin.ops.write"
)

var AdminAPIKeyScopes = []string{
	AdminAPIKeyScopeRead, AdminAPIKeyScopeWrite,
	AdminAPIKeyScopeUsersRead, AdminAPIKeyScopeUsersWrite,
	AdminAPIKeyScopeAccountsRead, AdminAPIKeyScopeAccountsWrite,
	AdminAPIKeyScopeSettingsRead, AdminAPIKeyScopeSettingsWrite,
	AdminAPIKeyScopeBackupsRead, AdminAPIKeyScopeBackupsWrite,
	AdminAPIKeyScopeSystemRead, AdminAPIKeyScopeSystemWrite,
	AdminAPIKeyScopeAuditRead, AdminAPIKeyScopeAuditWrite,
	AdminAPIKeyScopeOpsRead, AdminAPIKeyScopeOpsWrite,
}

type AdminAPIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastFour   string     `json:"last_four"`
	Scopes     []string   `json:"scopes"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedBy  int64      `json:"created_by"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type AdminAPIKeyCreateInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type AdminAPIKeyUpdateInput struct {
	Name      *string
	Scopes    *[]string
	ExpiresAt **time.Time
}

type adminAPIKeyRecord struct {
	AdminAPIKey
	KeyHash string `json:"key_hash"`
}

type adminAPIKeyStore struct {
	Version int                 `json:"version"`
	Keys    []adminAPIKeyRecord `json:"keys"`
}

func normalizeAdminAPIKeyScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{AdminAPIKeyScopeRead}, nil
	}
	allowed := make(map[string]struct{}, len(AdminAPIKeyScopes))
	for _, scope := range AdminAPIKeyScopes {
		allowed[scope] = struct{}{}
	}
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		scope := strings.TrimSpace(raw)
		if scope == "" {
			continue
		}
		if _, ok := allowed[scope]; !ok {
			return nil, fmt.Errorf("unknown admin API key scope: %s", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	if len(result) == 0 {
		return []string{AdminAPIKeyScopeRead}, nil
	}
	return result, nil
}

func adminAPIKeyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func generateAdminAPIKeySecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return AdminAPIKeyPrefix + hex.EncodeToString(bytes), nil
}

func (s *SettingService) readAdminAPIKeyStore(ctx context.Context) (*adminAPIKeyStore, error) {
	value, err := s.settingRepo.GetValue(ctx, adminAPIKeysSetting)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return &adminAPIKeyStore{Version: 1, Keys: []adminAPIKeyRecord{}}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return &adminAPIKeyStore{Version: 1, Keys: []adminAPIKeyRecord{}}, nil
	}
	var store adminAPIKeyStore
	if err := json.Unmarshal([]byte(value), &store); err != nil {
		return nil, fmt.Errorf("decode admin API key store: %w", err)
	}
	if store.Version == 0 {
		store.Version = 1
	}
	if store.Keys == nil {
		store.Keys = []adminAPIKeyRecord{}
	}
	return &store, nil
}

func (s *SettingService) writeAdminAPIKeyStore(ctx context.Context, store *adminAPIKeyStore) error {
	data, err := json.Marshal(store)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, adminAPIKeysSetting, string(data))
}

func cloneAdminAPIKey(record *adminAPIKeyRecord) AdminAPIKey {
	key := record.AdminAPIKey
	key.Scopes = append([]string(nil), record.Scopes...)
	return key
}

func (s *SettingService) ListAdminAPIKeys(ctx context.Context) ([]AdminAPIKey, error) {
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AdminAPIKey, 0, len(store.Keys)+1)
	for i := range store.Keys {
		result = append(result, cloneAdminAPIKey(&store.Keys[i]))
	}
	// Preserve the pre-scoped single-key setting as a read-only legacy entry.
	if legacy, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey); err == nil && legacy != "" {
		now := time.Now().UTC()
		result = append(result, AdminAPIKey{ID: "legacy", Name: "Legacy Admin API Key", KeyPrefix: legacy[:adminMinInt(10, len(legacy))], LastFour: legacy[adminMaxInt(0, len(legacy)-4):], Scopes: []string{AdminAPIKeyScopeRead}, Status: "active", CreatedAt: now, UpdatedAt: now})
	} else if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	return result, nil
}

func (s *SettingService) CreateAdminAPIKey(ctx context.Context, input AdminAPIKeyCreateInput, createdBy int64) (AdminAPIKey, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 100 {
		return AdminAPIKey{}, "", errors.New("name must be between 1 and 100 characters")
	}
	scopes, err := normalizeAdminAPIKeyScopes(input.Scopes)
	if err != nil {
		return AdminAPIKey{}, "", err
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		return AdminAPIKey{}, "", errors.New("expires_at must be in the future")
	}
	secret, err := generateAdminAPIKeySecret()
	if err != nil {
		return AdminAPIKey{}, "", fmt.Errorf("generate admin API key: %w", err)
	}
	now := time.Now().UTC()
	key := AdminAPIKey{ID: uuid.NewString(), Name: name, KeyPrefix: secret[:10], LastFour: secret[len(secret)-4:], Scopes: scopes, Status: "active", ExpiresAt: input.ExpiresAt, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now}
	record := adminAPIKeyRecord{AdminAPIKey: key, KeyHash: adminAPIKeyDigest(secret)}
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return AdminAPIKey{}, "", err
	}
	store.Keys = append(store.Keys, record)
	if err := s.writeAdminAPIKeyStore(ctx, store); err != nil {
		return AdminAPIKey{}, "", err
	}
	return key, secret, nil
}

func (s *SettingService) UpdateAdminAPIKey(ctx context.Context, id string, input AdminAPIKeyUpdateInput) (AdminAPIKey, error) {
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return AdminAPIKey{}, err
	}
	for i := range store.Keys {
		if store.Keys[i].ID != id {
			continue
		}
		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if name == "" || len(name) > 100 {
				return AdminAPIKey{}, errors.New("name must be between 1 and 100 characters")
			}
			store.Keys[i].Name = name
		}
		if input.Scopes != nil {
			scopes, scopeErr := normalizeAdminAPIKeyScopes(*input.Scopes)
			if scopeErr != nil {
				return AdminAPIKey{}, scopeErr
			}
			store.Keys[i].Scopes = scopes
		}
		if input.ExpiresAt != nil {
			if *input.ExpiresAt != nil && !(*input.ExpiresAt).After(time.Now()) {
				return AdminAPIKey{}, errors.New("expires_at must be in the future")
			}
			store.Keys[i].ExpiresAt = *input.ExpiresAt
		}
		store.Keys[i].UpdatedAt = time.Now().UTC()
		if err := s.writeAdminAPIKeyStore(ctx, store); err != nil {
			return AdminAPIKey{}, err
		}
		return cloneAdminAPIKey(&store.Keys[i]), nil
	}
	return AdminAPIKey{}, ErrSettingNotFound
}

func (s *SettingService) RotateAdminAPIKey(ctx context.Context, id string) (AdminAPIKey, string, error) {
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return AdminAPIKey{}, "", err
	}
	for i := range store.Keys {
		if store.Keys[i].ID != id {
			continue
		}
		secret, generateErr := generateAdminAPIKeySecret()
		if generateErr != nil {
			return AdminAPIKey{}, "", generateErr
		}
		store.Keys[i].KeyHash = adminAPIKeyDigest(secret)
		store.Keys[i].KeyPrefix = secret[:10]
		store.Keys[i].LastFour = secret[len(secret)-4:]
		store.Keys[i].Status = "active"
		store.Keys[i].RevokedAt = nil
		store.Keys[i].UpdatedAt = time.Now().UTC()
		if err := s.writeAdminAPIKeyStore(ctx, store); err != nil {
			return AdminAPIKey{}, "", err
		}
		return cloneAdminAPIKey(&store.Keys[i]), secret, nil
	}
	return AdminAPIKey{}, "", ErrSettingNotFound
}

func (s *SettingService) RevokeAdminAPIKey(ctx context.Context, id string) error {
	if id == "legacy" {
		return s.DeleteAdminAPIKey(ctx)
	}
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return err
	}
	for i := range store.Keys {
		if store.Keys[i].ID == id {
			now := time.Now().UTC()
			store.Keys[i].Status = "revoked"
			store.Keys[i].RevokedAt = &now
			store.Keys[i].UpdatedAt = now
			return s.writeAdminAPIKeyStore(ctx, store)
		}
	}
	return ErrSettingNotFound
}

func (s *SettingService) AuthenticateAdminAPIKey(ctx context.Context, key string) (*AdminAPIKey, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("invalid admin API key")
	}
	digest := adminAPIKeyDigest(key)
	s.adminAPIKeyMu.Lock()
	defer s.adminAPIKeyMu.Unlock()
	store, err := s.readAdminAPIKeyStore(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range store.Keys {
		record := &store.Keys[i]
		if subtle.ConstantTimeCompare([]byte(record.KeyHash), []byte(digest)) != 1 {
			continue
		}
		if record.Status != "active" || record.RevokedAt != nil || (record.ExpiresAt != nil && !record.ExpiresAt.After(now)) {
			return nil, errors.New("admin API key is inactive or expired")
		}
		record.LastUsedAt = &now
		record.UpdatedAt = now
		if err := s.writeAdminAPIKeyStore(ctx, store); err != nil {
			return nil, err
		}
		keyCopy := cloneAdminAPIKey(record)
		return &keyCopy, nil
	}
	// Backwards-compatible legacy key. It remains read-only until explicitly
	// removed from the settings panel.
	legacy, legacyErr := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if legacyErr == nil && subtle.ConstantTimeCompare([]byte(key), []byte(legacy)) == 1 {
		return &AdminAPIKey{ID: "legacy", Name: "Legacy Admin API Key", KeyPrefix: legacy[:adminMinInt(10, len(legacy))], LastFour: legacy[adminMaxInt(0, len(legacy)-4):], Scopes: []string{AdminAPIKeyScopeRead}, Status: "active", CreatedAt: now, UpdatedAt: now}, nil
	}
	if legacyErr != nil && !errors.Is(legacyErr, ErrSettingNotFound) {
		return nil, legacyErr
	}
	return nil, errors.New("invalid admin API key")
}

func adminMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func adminMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
