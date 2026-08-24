package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
	"github.com/stretchr/testify/require"
)

type remoteControlStoreRepo struct {
	AccountRepository
	account *Account
	updates map[string]any
}

func (r *remoteControlStoreRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *remoteControlStoreRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		if value == nil {
			delete(r.account.Extra, key)
		} else {
			r.account.Extra[key] = value
		}
	}
	return nil
}

type remoteControlStoreEncryptor struct{}

func (remoteControlStoreEncryptor) Encrypt(value string) (string, error) { return "enc:" + value, nil }
func (remoteControlStoreEncryptor) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "enc:"), nil
}

func TestRemoteControlAccountEnrollmentStoreEncryptsToken(t *testing.T) {
	account := &Account{ID: 7, Extra: map[string]any{}}
	repo := &remoteControlStoreRepo{account: account}
	store := NewRemoteControlAccountEnrollmentStore(repo, remoteControlStoreEncryptor{}, account.ID)
	wantExpiry := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	record := remotecontrol.StoredEnrollment{Enrollment: remotecontrol.Enrollment{
		ServerID: "server-1", EnvironmentID: "env-1", ServerName: "host", RemoteControlToken: "secret-token",
	}, ExpiresAt: wantExpiry}
	require.NoError(t, store.Save(context.Background(), record))
	require.Equal(t, "enc:secret-token", account.Extra[CodexRemoteControlTokenCiphertextExtraKey])
	loaded, err := store.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, "secret-token", loaded.RemoteControlToken)
	require.Equal(t, wantExpiry, loaded.ExpiresAt)
	require.NoError(t, store.Clear(context.Background()))
	_, err = store.Load(context.Background())
	require.NoError(t, err)
}
