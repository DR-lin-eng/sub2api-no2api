//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeusage"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

// The registration service opens the outer transaction and the two repositories
// must join it. A rollback must remove both the user and the usage row; this is
// the regression boundary for the invitation TOCTOU fix.
func TestUserRepositoryCreateAndRedeemUseJoinOuterTransaction(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)

	codeValue := "ITX-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	codeEntity, err := client.RedeemCode.Create().
		SetCode(codeValue).
		SetType(service.RedeemTypeInvitation).
		SetStatus(service.StatusUnused).
		SetValue(0).
		SetMaxUses(2).
		SetMaxUsesPerUser(1).
		Save(ctx)
	require.NoError(t, err)

	email := uniqueTestValue(t, "invite-user") + "@example.com"
	t.Cleanup(func() {
		_, _ = client.RedeemCodeUsage.Delete().Where(redeemcodeusage.RedeemCodeIDEQ(codeEntity.ID)).Exec(ctx)
		_, _ = client.RedeemCode.Delete().Where(redeemcode.IDEQ(codeEntity.ID)).Exec(ctx)
		_, _ = client.User.Delete().Where(user.EmailEQ(email)).Exec(ctx)
	})

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	created := &service.User{
		Email:        email,
		PasswordHash: "test-password-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  1,
	}
	require.NoError(t, userRepo.CreateWithEmailAliasGuard(txCtx, created))
	require.NoError(t, redeemRepo.Use(txCtx, codeEntity.ID, created.ID))
	require.NoError(t, tx.Rollback())

	exists, err := client.User.Query().Where(user.EmailEQ(email)).Exist(ctx)
	require.NoError(t, err)
	require.False(t, exists, "outer rollback must remove the newly created user")

	codeAfter, err := client.RedeemCode.Get(ctx, codeEntity.ID)
	require.NoError(t, err)
	require.Zero(t, codeAfter.UsedCount, "outer rollback must release the invitation usage")
	usageCount, err := client.RedeemCodeUsage.Query().Where(redeemcodeusage.RedeemCodeIDEQ(codeEntity.ID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, usageCount, "outer rollback must remove the usage record")
}
