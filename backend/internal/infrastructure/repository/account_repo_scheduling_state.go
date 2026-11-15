package repository

import (
	"context"

	dbaccount "github.com/DR-lin-eng/sub2api-no2api/ent/account"
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
)

func (r *accountRepository) GetAccountSchedulingState(ctx context.Context, accountID int64) (service.AccountSchedulingState, error) {
	account, err := r.client.Account.Query().
		Where(dbaccount.IDEQ(accountID)).
		Select(
			dbaccount.FieldID,
			dbaccount.FieldPlatform,
			dbaccount.FieldType,
			dbaccount.FieldStatus,
			dbaccount.FieldSchedulable,
			dbaccount.FieldExpiresAt,
			dbaccount.FieldAutoPauseOnExpired,
			dbaccount.FieldOverloadUntil,
			dbaccount.FieldRateLimitResetAt,
			dbaccount.FieldTempUnschedulableUntil,
			dbaccount.FieldTempUnschedulableReason,
			dbaccount.FieldExtra,
		).
		Only(ctx)
	if err != nil {
		return service.AccountSchedulingState{}, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	projection := service.Account{
		ID:                      account.ID,
		Platform:                account.Platform,
		Type:                    account.Type,
		Status:                  account.Status,
		Schedulable:             account.Schedulable,
		ExpiresAt:               account.ExpiresAt,
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		OverloadUntil:           account.OverloadUntil,
		RateLimitResetAt:        account.RateLimitResetAt,
		TempUnschedulableUntil:  account.TempUnschedulableUntil,
		TempUnschedulableReason: derefString(account.TempUnschedulableReason),
		Extra:                   account.Extra,
	}
	return service.AccountSchedulingState{
		Exists:      true,
		Schedulable: projection.IsSchedulable(),
	}, nil
}
