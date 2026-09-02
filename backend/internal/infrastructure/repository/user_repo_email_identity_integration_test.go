//go:build integration

package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (s *UserRepoSuite) TestCreate_CreatesEmailAuthIdentityForNormalEmail() {
	user := &service.User{
		Email:        "repo-create@example.com",
		PasswordHash: "test-password-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  2,
	}

	s.Require().NoError(s.repo.Create(s.ctx, user))

	identity, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("repo-create@example.com"),
		).
		Only(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(user.ID, identity.UserID)
}

func (s *UserRepoSuite) TestCreate_SkipsEmailAuthIdentityForSyntheticLinuxDoEmail() {
	user := &service.User{
		Email:        "linuxdo-legacy-user@linuxdo-connect.invalid",
		PasswordHash: "test-password-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  2,
	}

	s.Require().NoError(s.repo.Create(s.ctx, user))

	count, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
		).
		Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count)
}

func (s *UserRepoSuite) TestUpdate_ReplacesEmailAuthIdentityWhenEmailChanges() {
	user := s.mustCreateUser(&service.User{
		Email: "before-update@example.com",
	})

	user.Email = "after-update@example.com"
	s.Require().NoError(s.repo.Update(s.ctx, user, service.UserUpdateFields{Email: true}))

	newIdentity, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("after-update@example.com"),
		).
		Only(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(user.ID, newIdentity.UserID)

	oldCount, err := s.client.AuthIdentity.Query().
		Where(
			authidentity.UserIDEQ(user.ID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ("before-update@example.com"),
		).
		Count(context.Background())
	s.Require().NoError(err)
	s.Require().Zero(oldCount)
}

func (s *UserRepoSuite) TestUpdateEmailWithAliasGuardRejectsAliasCollision() {
	owner := s.mustCreateUser(&service.User{Email: "alice@gmail.com"})
	target := s.mustCreateUser(&service.User{Email: "alias-target@synthetic.invalid"})

	tx, err := s.client.Tx(s.ctx)
	s.Require().NoError(err)
	txCtx := dbent.NewTxContext(s.ctx, tx)
	err = s.repo.UpdateEmailWithAliasGuard(txCtx, target.ID, "a.l.i.c.e+tag@googlemail.com", "new-password-hash")
	s.Require().ErrorIs(err, service.ErrEmailExists)
	s.Require().NoError(tx.Rollback())

	stored, err := s.client.User.Get(s.ctx, target.ID)
	s.Require().NoError(err)
	s.Require().Equal("alias-target@synthetic.invalid", stored.Email)
	s.Require().NotEqual(owner.ID, target.ID)
}
