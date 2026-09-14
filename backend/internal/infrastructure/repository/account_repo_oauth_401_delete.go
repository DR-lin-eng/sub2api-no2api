package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbbinding "github.com/Wei-Shaw/sub2api/ent/accountegressbinding"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/lib/pq"
)

// DeleteOAuthAccountIfCredentialsUnchanged atomically soft-deletes a direct
// OAuth credential owner and its linked shadows only while the complete JSONB
// credential document still matches the request that received a 401.
func (r *accountRepository) DeleteOAuthAccountIfCredentialsUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
) ([]int64, bool, error) {
	if r == nil || r.client == nil || id <= 0 {
		return nil, false, errors.New("account repository is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return nil, false, fmt.Errorf("marshal expected OAuth credentials: %w", err)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, false, err
	}
	txClient := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	}

	matched, err := queryLockedAccountIDs(ctx, txClient, `
		SELECT id
		FROM accounts
		WHERE id = $1
			AND deleted_at IS NULL
			AND parent_account_id IS NULL
			AND type = $2
			AND credentials = $3::jsonb
		FOR UPDATE
	`, id, service.AccountTypeOAuth, string(expectedJSON))
	if err != nil {
		return nil, false, err
	}
	if len(matched) == 0 {
		return nil, false, nil
	}

	shadowIDs, err := queryLockedAccountIDs(ctx, txClient, `
		SELECT id
		FROM accounts
		WHERE parent_account_id = $1 AND deleted_at IS NULL
		ORDER BY id
		FOR UPDATE
	`, id)
	if err != nil {
		return nil, false, err
	}
	deletedIDs := append(shadowIDs, id)

	groupIDsByAccount := make(map[int64][]int64, len(deletedIDs))
	bindings, err := txClient.AccountGroup.Query().
		Where(dbaccountgroup.AccountIDIn(deletedIDs...)).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, binding := range bindings {
		groupIDsByAccount[binding.AccountID] = append(groupIDsByAccount[binding.AccountID], binding.GroupID)
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDIn(deletedIDs...)).Exec(ctx); err != nil {
		return nil, false, err
	}
	if _, err := txClient.AccountEgressBinding.Delete().Where(dbbinding.AccountIDIn(deletedIDs...)).Exec(ctx); err != nil {
		return nil, false, err
	}
	if _, err := txClient.ExecContext(ctx, "DELETE FROM scheduled_test_plans WHERE account_id = ANY($1)", pq.Array(deletedIDs)); err != nil {
		return nil, false, err
	}
	deleted, err := txClient.Account.Delete().Where(dbaccount.IDIn(deletedIDs...)).Exec(ctx)
	if err != nil {
		return nil, false, err
	}
	if deleted != len(deletedIDs) {
		return nil, false, fmt.Errorf("OAuth 401 delete affected %d accounts, expected %d", deleted, len(deletedIDs))
	}
	for _, accountID := range deletedIDs {
		if err := enqueueSchedulerOutbox(
			ctx,
			txClient,
			service.SchedulerOutboxEventAccountChanged,
			&accountID,
			nil,
			buildSchedulerGroupPayload(groupIDsByAccount[accountID]),
		); err != nil {
			return nil, false, err
		}
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	for _, accountID := range deletedIDs {
		r.deleteSchedulerAccountSnapshot(cacheCtx, accountID)
	}
	return deletedIDs, true, nil
}

func queryLockedAccountIDs(ctx context.Context, client *dbent.Client, query string, args ...any) ([]int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
