//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPasskeyRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, testEntClient(t), &service.User{
		Email: "passkey-" + uuid.NewString() + "@example.com",
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	repo := NewPasskeyRepository(integrationDB)
	firstCandidate := []byte("0123456789abcdef0123456789abcdef")
	handle, err := repo.EnsureUserHandle(ctx, user.ID, firstCandidate)
	require.NoError(t, err)
	require.Equal(t, firstCandidate, handle)

	secondCandidate := []byte("fedcba9876543210fedcba9876543210")
	handle, err = repo.EnsureUserHandle(ctx, user.ID, secondCandidate)
	require.NoError(t, err)
	require.Equal(t, firstCandidate, handle, "an existing WebAuthn user handle must remain stable")

	credential := webauthn.Credential{ID: []byte("credential-" + uuid.NewString())}
	created, err := repo.Create(ctx, &service.PasskeyCredentialRecord{
		UserID:     user.ID,
		UserHandle: handle,
		Name:       "Laptop",
		Credential: credential,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "Laptop", created.Name)

	loaded, err := repo.GetByCredentialID(ctx, credential.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)
	require.Equal(t, handle, loaded.UserHandle)

	listed, err := repo.ListByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)

	usedAt := time.Now().UTC().Truncate(time.Microsecond)
	credential.Flags.BackupState = true
	require.NoError(t, repo.UpdateCredential(ctx, user.ID, &credential, usedAt))
	loaded, err = repo.GetByCredentialID(ctx, credential.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded.LastUsedAt)
	require.WithinDuration(t, usedAt, *loaded.LastUsedAt, time.Microsecond)
	require.True(t, loaded.Credential.Flags.BackupState)

	require.NoError(t, repo.Rename(ctx, user.ID, created.ID, "Security key"))
	listed, err = repo.ListByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "Security key", listed[0].Name)

	require.NoError(t, repo.Delete(ctx, user.ID, created.ID))
	_, err = repo.GetByCredentialID(ctx, credential.ID)
	require.ErrorIs(t, err, service.ErrPasskeyNotFound)
}
