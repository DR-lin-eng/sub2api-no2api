package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
)

// RevokeOpenAIWorkspaceAccounts disables direct OAuth owners of this workspace
// and their shadows. Workspace matching and outbox persistence share one atomic
// statement, so a concurrent move to another workspace cannot be overwritten
// from an earlier application snapshot.
func (r *accountRepository) RevokeOpenAIWorkspaceAccounts(ctx context.Context, workspaceID, errorMessage string) ([]int64, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("account repository SQL executor is not configured")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH workspace_owners AS MATERIALIZED (
			SELECT id FROM accounts
			WHERE deleted_at IS NULL AND parent_account_id IS NULL
				AND platform = $4 AND type = $5
				AND BTRIM(credentials->>'chatgpt_account_id') = $3
			ORDER BY id FOR UPDATE
		), updated AS (
			UPDATE accounts AS a
			SET status = $1, error_message = $2, schedulable = FALSE, updated_at = NOW()
			WHERE a.deleted_at IS NULL AND a.platform = $4
				AND (a.id IN (SELECT id FROM workspace_owners)
					OR a.parent_account_id IN (SELECT id FROM workspace_owners))
			RETURNING a.id
		), outbox AS (
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
			SELECT $6, updated.id, NULL, NULL FROM updated
			RETURNING account_id
		)
		SELECT account_id FROM outbox ORDER BY account_id
	`, service.StatusError, errorMessage, workspaceID, service.PlatformOpenAI, service.AccountTypeOAuth, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var accountIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		accountIDs = append(accountIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	r.syncSchedulerAccountSnapshots(cacheCtx, accountIDs)
	return accountIDs, nil
}
