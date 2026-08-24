package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
)

const (
	CodexRemoteControlServerIDExtraKey        = "codex_remote_control_server_id"
	CodexRemoteControlEnvironmentIDExtraKey   = "codex_remote_control_environment_id"
	CodexRemoteControlServerNameExtraKey      = "codex_remote_control_server_name"
	CodexRemoteControlTokenCiphertextExtraKey = "codex_remote_control_token_ciphertext"
	CodexRemoteControlExpiresAtExtraKey       = "codex_remote_control_expires_at"
)

// RemoteControlAccountEnrollmentStore persists only encrypted server tokens
// in Account.Extra. It adapts one account row to remotecontrol.EnrollmentStore.
type RemoteControlAccountEnrollmentStore struct {
	repo      AccountRepository
	encryptor SecretEncryptor
	accountID int64
}

func NewRemoteControlAccountEnrollmentStore(repo AccountRepository, encryptor SecretEncryptor, accountID int64) *RemoteControlAccountEnrollmentStore {
	return &RemoteControlAccountEnrollmentStore{repo: repo, encryptor: encryptor, accountID: accountID}
}

func (s *RemoteControlAccountEnrollmentStore) Load(ctx context.Context) (*remotecontrol.StoredEnrollment, error) {
	if s == nil || s.repo == nil || s.accountID <= 0 {
		return nil, nil
	}
	account, err := s.repo.GetByID(ctx, s.accountID)
	if err != nil || account == nil {
		return nil, err
	}
	serverID := strings.TrimSpace(account.GetExtraString(CodexRemoteControlServerIDExtraKey))
	tokenCiphertext := strings.TrimSpace(account.GetExtraString(CodexRemoteControlTokenCiphertextExtraKey))
	if serverID == "" || tokenCiphertext == "" {
		return nil, nil
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("remote control enrollment encryptor is unavailable")
	}
	token, err := s.encryptor.Decrypt(tokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt remote control enrollment token: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(account.GetExtraString(CodexRemoteControlExpiresAtExtraKey)))
	if err != nil {
		return nil, fmt.Errorf("parse remote control enrollment expiry: %w", err)
	}
	return &remotecontrol.StoredEnrollment{Enrollment: remotecontrol.Enrollment{
		ServerID:           serverID,
		EnvironmentID:      account.GetExtraString(CodexRemoteControlEnvironmentIDExtraKey),
		ServerName:         account.GetExtraString(CodexRemoteControlServerNameExtraKey),
		RemoteControlToken: token,
	}, ExpiresAt: expiresAt}, nil
}

func (s *RemoteControlAccountEnrollmentStore) Save(ctx context.Context, enrollment remotecontrol.StoredEnrollment) error {
	if s == nil || s.repo == nil || s.accountID <= 0 || s.encryptor == nil {
		return fmt.Errorf("remote control enrollment store is unavailable")
	}
	ciphertext, err := s.encryptor.Encrypt(enrollment.RemoteControlToken)
	if err != nil {
		return fmt.Errorf("encrypt remote control enrollment token: %w", err)
	}
	return s.repo.UpdateExtra(ctx, s.accountID, map[string]any{
		CodexRemoteControlServerIDExtraKey:        enrollment.ServerID,
		CodexRemoteControlEnvironmentIDExtraKey:   enrollment.EnvironmentID,
		CodexRemoteControlServerNameExtraKey:      enrollment.ServerName,
		CodexRemoteControlTokenCiphertextExtraKey: ciphertext,
		CodexRemoteControlExpiresAtExtraKey:       enrollment.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *RemoteControlAccountEnrollmentStore) Clear(ctx context.Context) error {
	if s == nil || s.repo == nil || s.accountID <= 0 {
		return nil
	}
	return s.repo.UpdateExtra(ctx, s.accountID, map[string]any{
		CodexRemoteControlServerIDExtraKey:        nil,
		CodexRemoteControlEnvironmentIDExtraKey:   nil,
		CodexRemoteControlServerNameExtraKey:      nil,
		CodexRemoteControlTokenCiphertextExtraKey: nil,
		CodexRemoteControlExpiresAtExtraKey:       nil,
	})
}
