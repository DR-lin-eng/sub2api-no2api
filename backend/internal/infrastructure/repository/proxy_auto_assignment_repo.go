package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/application/service"

	entsql "entgo.io/ent/dialect/sql"
)

const proxyHealthErrorMaxBytes = 1000

func (r *proxyRepository) SelectLeastLoadedActiveProxy(ctx context.Context, now time.Time) (*int64, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		if errors.Is(err, dbent.ErrTxStarted) {
			return r.selectNextActiveProxyOnExec(ctx, r.sql, now)
		}
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	proxyID, err := r.selectNextActiveProxyOnExec(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return proxyID, nil
}

func (r *proxyRepository) selectNextActiveProxyOnExec(ctx context.Context, exec sqlExecutor, now time.Time) (*int64, error) {
	rows, err := exec.QueryContext(ctx, `
		SELECT id
		FROM proxies
		WHERE deleted_at IS NULL
			AND status = $1
			AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY id
		FOR NO KEY UPDATE
	`, service.StatusActive, now.UTC())
	if err != nil {
		return nil, err
	}
	proxyIDs := make([]int64, 0)
	for rows.Next() {
		var proxyID int64
		if err := rows.Scan(&proxyID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		proxyIDs = append(proxyIDs, proxyID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(proxyIDs) == 0 {
		return nil, service.ErrProxyAutoAssignmentNoActiveProxy
	}

	var parentCount int64
	if err := scanSingleRow(ctx, exec, `
		SELECT COUNT(*) FROM accounts WHERE deleted_at IS NULL AND parent_account_id IS NULL
	`, nil, &parentCount); err != nil {
		return nil, err
	}
	initialCursor := strconv.FormatInt(parentCount, 10)
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO NOTHING
	`, service.SettingKeyProxyAutoAssignmentCursor, initialCursor); err != nil {
		return nil, err
	}
	cursorRows, err := exec.QueryContext(ctx, `
		SELECT value FROM settings WHERE key = $1 FOR UPDATE
	`, service.SettingKeyProxyAutoAssignmentCursor)
	if err != nil {
		return nil, err
	}
	if !cursorRows.Next() {
		_ = cursorRows.Close()
		return nil, service.ErrProxyAutoAssignmentUnavailable
	}
	var rawCursor string
	if err := cursorRows.Scan(&rawCursor); err != nil {
		_ = cursorRows.Close()
		return nil, err
	}
	if err := cursorRows.Close(); err != nil {
		return nil, err
	}
	cursor, parseErr := strconv.ParseInt(strings.TrimSpace(rawCursor), 10, 64)
	if parseErr != nil || cursor < parentCount {
		cursor = parentCount
	}
	selectedID := proxyIDs[cursor%int64(len(proxyIDs))]
	if _, err := exec.ExecContext(ctx, `
		UPDATE settings SET value = $1, updated_at = NOW() WHERE key = $2
	`, strconv.FormatInt(cursor+1, 10), service.SettingKeyProxyAutoAssignmentCursor); err != nil {
		return nil, err
	}
	return &selectedID, nil
}

func (r *proxyRepository) RebalanceActiveProxyAccounts(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		if errors.Is(err, dbent.ErrTxStarted) {
			return r.rebalanceActiveProxyAccountsOnExec(ctx, r.sql, now)
		}
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	changed, err := r.rebalanceActiveProxyAccountsOnExec(ctx, tx, now)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (r *proxyRepository) rebalanceActiveProxyAccountsOnExec(ctx context.Context, exec sqlExecutor, now time.Time) (int64, error) {
	locked, err := exec.QueryContext(ctx, `
		SELECT id
		FROM proxies
		WHERE deleted_at IS NULL
			AND status = $1
			AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY id
		FOR NO KEY UPDATE
	`, service.StatusActive, now.UTC())
	if err != nil {
		return 0, err
	}
	activeCount := 0
	for locked.Next() {
		activeCount++
	}
	if err := locked.Err(); err != nil {
		_ = locked.Close()
		return 0, err
	}
	if err := locked.Close(); err != nil {
		return 0, err
	}
	if activeCount == 0 {
		return 0, service.ErrProxyAutoAssignmentNoActiveProxy
	}

	rows, err := exec.QueryContext(ctx, `
		WITH eligible_proxies AS MATERIALIZED (
			SELECT id,
				ROW_NUMBER() OVER (ORDER BY id) AS slot,
				COUNT(*) OVER () AS total
			FROM proxies
			WHERE deleted_at IS NULL
				AND status = $1
				AND (expires_at IS NULL OR expires_at > $2)
		), ranked_accounts AS MATERIALIZED (
			SELECT id, ROW_NUMBER() OVER (ORDER BY id) AS slot
			FROM accounts
			WHERE deleted_at IS NULL AND parent_account_id IS NULL
		), parent_assignments AS MATERIALIZED (
			SELECT a.id AS account_id, p.id AS proxy_id
			FROM ranked_accounts AS a
			JOIN eligible_proxies AS p
				ON p.slot = MOD(a.slot - 1, p.total) + 1
		), target_assignments AS (
			SELECT account_id, proxy_id FROM parent_assignments
			UNION ALL
			SELECT child.id AS account_id, parent.proxy_id
			FROM accounts AS child
			JOIN parent_assignments AS parent ON parent.account_id = child.parent_account_id
			WHERE child.deleted_at IS NULL
		)
		UPDATE accounts AS account
		SET proxy_id = target.proxy_id,
			proxy_fallback_origin_id = NULL,
			extra = COALESCE(account.extra, '{}'::jsonb)
				- 'upstream_billing_probe'
				- 'ollama_cloud_usage_snapshot',
			updated_at = NOW()
		FROM target_assignments AS target
		WHERE account.id = target.account_id
			AND (
				account.proxy_id IS DISTINCT FROM target.proxy_id
				OR account.proxy_fallback_origin_id IS NOT NULL
			)
		RETURNING account.id
	`, service.StatusActive, now.UTC())
	if err != nil {
		return 0, err
	}
	changedAccountIDs := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		changedAccountIDs = append(changedAccountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := enqueueProxyProbeAccountChanges(ctx, exec, changedAccountIDs); err != nil {
		return 0, err
	}
	var parentCount int64
	if err := scanSingleRow(ctx, exec, `
		SELECT COUNT(*) FROM accounts WHERE deleted_at IS NULL AND parent_account_id IS NULL
	`, nil, &parentCount); err != nil {
		return 0, err
	}
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
	`, service.SettingKeyProxyAutoAssignmentCursor, strconv.FormatInt(parentCount, 10)); err != nil {
		return 0, err
	}
	return int64(len(changedAccountIDs)), nil
}

func (r *proxyRepository) ListDueProxyHealthChecks(ctx context.Context, cutoff, now time.Time, limit int) ([]service.Proxy, error) {
	if limit <= 0 {
		return []service.Proxy{}, nil
	}
	entities, err := r.client.Proxy.Query().
		Where(
			proxy.StatusEQ(service.StatusActive),
			proxy.Or(proxy.ExpiresAtIsNil(), proxy.ExpiresAtGT(now.UTC())),
			proxy.Or(proxy.LastHealthCheckAtIsNil(), proxy.LastHealthCheckAtLTE(cutoff.UTC())),
		).
		Order(
			entsql.OrderByField(proxy.FieldLastHealthCheckAt, entsql.OrderNullsFirst()).ToFunc(),
			dbent.Asc(proxy.FieldID),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.Proxy, 0, len(entities))
	for i := range entities {
		result = append(result, *proxyEntityToService(entities[i]))
	}
	return result, nil
}

func (r *proxyRepository) RecordProxyHealthCheck(ctx context.Context, proxyID int64, checkedAt time.Time, success bool, message string, failureThreshold int) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		if errors.Is(err, dbent.ErrTxStarted) {
			return r.recordProxyHealthCheckOnExec(ctx, r.sql, proxyID, checkedAt, success, message, failureThreshold)
		}
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	deactivated, err := r.recordProxyHealthCheckOnExec(ctx, tx, proxyID, checkedAt, success, message, failureThreshold)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return deactivated, nil
}

func (r *proxyRepository) recordProxyHealthCheckOnExec(ctx context.Context, exec sqlExecutor, proxyID int64, checkedAt time.Time, success bool, message string, failureThreshold int) (bool, error) {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT status, health_consecutive_failures
		FROM proxies
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, proxyID)
	if err != nil {
		return false, err
	}
	if !rows.Next() {
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, service.ErrProxyNotFound
	}
	var status string
	var failures int
	if err := rows.Scan(&status, &failures); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}

	if success {
		_, err = exec.ExecContext(ctx, `
			UPDATE proxies
			SET health_status = $1,
				health_consecutive_failures = 0,
				last_health_check_at = $2,
				last_health_error = NULL,
				updated_at = NOW()
			WHERE id = $3 AND deleted_at IS NULL
		`, service.ProxyHealthHealthy, checkedAt.UTC(), proxyID)
		return false, err
	}

	failures++
	healthStatus := service.ProxyHealthDegraded
	deactivated := status == service.StatusActive && failures >= failureThreshold
	if failures >= failureThreshold {
		healthStatus = service.ProxyHealthUnhealthy
	}
	message = strings.TrimSpace(message)
	if len(message) > proxyHealthErrorMaxBytes {
		message = message[:proxyHealthErrorMaxBytes]
	}
	newStatus := status
	if deactivated {
		newStatus = service.ProxyStatusInactive
	}
	if _, err := exec.ExecContext(ctx, `
		UPDATE proxies
		SET status = $1,
			health_status = $2,
			health_consecutive_failures = $3,
			last_health_check_at = $4,
			last_health_error = $5,
			updated_at = NOW()
		WHERE id = $6 AND deleted_at IS NULL
	`, newStatus, healthStatus, failures, checkedAt.UTC(), nullableHealthError(message), proxyID); err != nil {
		return false, err
	}
	if !deactivated {
		return false, nil
	}
	accountIDs, err := invalidateProxyProbeSnapshots(ctx, exec, proxyID)
	if err != nil {
		return false, err
	}
	if err := enqueueProxyProbeAccountChanges(ctx, exec, accountIDs); err != nil {
		return false, err
	}
	return true, nil
}

func nullableHealthError(message string) any {
	if message == "" {
		return nil
	}
	return message
}

var (
	_ service.ProxyAutoAssignmentRepository = (*proxyRepository)(nil)
	_ service.ProxyHealthRepository         = (*proxyRepository)(nil)
)
