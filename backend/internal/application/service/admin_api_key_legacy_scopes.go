package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const legacyAdminKeyOptionsSetting = "legacy_admin_api_key_options"

// Options are bound to the current secret's digest. Existing software can keep
// its legacy header value; rotating that value does not inherit old privileges.
func (s *SettingService) legacyAdminKeyRecord(ctx context.Context, secret string) (*adminAPIKeyRecord, error) {
	now := time.Now().UTC()
	record := &adminAPIKeyRecord{AdminAPIKey: AdminAPIKey{ID: "legacy", Name: "Legacy Admin API Key",
		KeyPrefix: secret[:adminMinInt(10, len(secret))], LastFour: secret[adminMaxInt(0, len(secret)-4):],
		Scopes: []string{AdminAPIKeyScopeRead}, Status: "active", CreatedAt: now, UpdatedAt: now}, KeyHash: adminAPIKeyDigest(secret)}
	value, err := s.settingRepo.GetValue(ctx, legacyAdminKeyOptionsSetting)
	if errors.Is(err, ErrSettingNotFound) || err == nil && strings.TrimSpace(value) == "" {
		return record, nil
	}
	if err != nil {
		return nil, err
	}
	var options adminAPIKeyRecord
	if err := json.Unmarshal([]byte(value), &options); err != nil {
		return nil, err
	}
	if options.KeyHash != record.KeyHash {
		return record, nil
	}
	if _, err := normalizeAdminAPIKeyScopes(options.Scopes); err != nil {
		return nil, err
	}
	options.ID = "legacy"
	options.KeyPrefix, options.LastFour = record.KeyPrefix, record.LastFour
	return &options, nil
}

func (s *SettingService) updateLegacyAdminKeyOptions(ctx context.Context, input AdminAPIKeyUpdateInput) (AdminAPIKey, error) {
	secret, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		return AdminAPIKey{}, err
	}
	if secret == "" {
		return AdminAPIKey{}, ErrSettingNotFound
	}
	record, err := s.legacyAdminKeyRecord(ctx, secret)
	if err != nil {
		return AdminAPIKey{}, err
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len(name) > 100 {
			return AdminAPIKey{}, errors.New("name must be between 1 and 100 characters")
		}
		record.Name = name
	}
	if input.Scopes != nil {
		scopes, err := normalizeAdminAPIKeyScopes(*input.Scopes)
		if err != nil {
			return AdminAPIKey{}, err
		}
		record.Scopes = scopes
	}
	if input.ExpiresAt != nil {
		if *input.ExpiresAt != nil && !(*input.ExpiresAt).After(time.Now()) {
			return AdminAPIKey{}, errors.New("expires_at must be in the future")
		}
		record.ExpiresAt = *input.ExpiresAt
	}
	record.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(record)
	if err != nil {
		return AdminAPIKey{}, err
	}
	if err := s.settingRepo.Set(ctx, legacyAdminKeyOptionsSetting, string(raw)); err != nil {
		return AdminAPIKey{}, err
	}
	return cloneAdminAPIKey(record), nil
}
