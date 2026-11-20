package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/migrationhttp"
)

// Only keys bearing an explicit migration permission need the peer public key.
func (s *SettingService) generateScopedKey(scopes []string) (string, error) {
	secret, e := generateAdminAPIKeySecret()
	if e != nil {
		return "", e
	}
	for _, scope := range scopes {
		if strings.HasPrefix(scope, "admin.migration.") && s.cfg != nil && len(s.cfg.MigrationHTTPRoot) == 32 {
			return migrationhttp.PinKey(secret, s.cfg.MigrationHTTPRoot)
		}
	}
	return secret, nil
}

func (s *SettingService) OpenMigrationHTTP(req *http.Request, frame []byte) (migrationhttp.Opened, error) {
	if s == nil || s.cfg == nil {
		return migrationhttp.Opened{}, migrationhttp.ErrEnvelope
	}
	return migrationhttp.OpenRequest(req, s.cfg.MigrationHTTPRoot, frame, time.Now())
}
func (s *SettingService) ConsumeMigrationHTTP(ctx context.Context, opened migrationhttp.Opened) error {
	store, ok := s.settingRepo.(interface {
		ConsumeMigrationHTTP(context.Context, []byte, time.Time) error
	})
	if !ok {
		return errors.New("migration replay store unavailable")
	}
	digest := sha256.Sum256(opened.Nonce)
	return store.ConsumeMigrationHTTP(ctx, digest[:], opened.Expires)
}
