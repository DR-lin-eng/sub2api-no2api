package service

import (
	"context"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// createEmailUserAndClaimInvitation keeps user creation and invitation usage in
// one database transaction. Invitation validation happens before this method,
// but the final Use call is deliberately repeated inside the transaction so a
// concurrent registration cannot leave an account behind when the code limit is
// reached. RedeemCodeRepository.Use keeps the existing max_uses and
// max_uses_per_user semantics; this method only supplies the transaction.
func (s *AuthService) createEmailUserAndClaimInvitation(
	ctx context.Context,
	user *User,
	invitation *RedeemCode,
) error {
	if s == nil || s.userRepo == nil || user == nil {
		return ErrServiceUnavailable
	}

	createAndClaim := func(execCtx context.Context) error {
		if err := s.userRepo.CreateWithEmailAliasGuard(execCtx, user); err != nil {
			return err
		}
		if invitation == nil {
			return nil
		}
		if s.redeemRepo == nil {
			return ErrServiceUnavailable
		}
		if err := s.redeemRepo.Use(execCtx, invitation.ID, user.ID); err != nil {
			if errors.Is(err, ErrRedeemCodeUsed) || errors.Is(err, ErrRedeemCodeNotFound) || errors.Is(err, ErrRedeemCodeExpired) {
				return ErrInvitationCodeInvalid
			}
			return fmt.Errorf("claim invitation code: %w", err)
		}
		return nil
	}

	if invitation == nil {
		return createAndClaim(ctx)
	}

	// A production AuthService is always wired with the Ent client. Fail closed
	// for incomplete test/standalone wiring instead of recreating the old
	// non-atomic path that could leave an orphan account.
	if s.entClient == nil {
		return ErrServiceUnavailable
	}
	if dbent.TxFromContext(ctx) != nil {
		return createAndClaim(ctx)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := createAndClaim(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit registration transaction: %w", err)
	}
	return nil
}
