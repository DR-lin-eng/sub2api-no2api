//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestListDueUpstreamBillingProbeAccountsHandlesInvalidCalendarDate(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = extra - 'upstream_billing_probe_enabled' - 'upstream_billing_probe'
	`)
	require.NoError(t, err)

	insert := func(name, nextProbeAt string, nextProbeUnix *int64) int64 {
		t.Helper()
		var id int64
		nextProbeUnixField := ""
		if nextProbeUnix != nil {
			nextProbeUnixField = fmt.Sprintf(`, "next_probe_unix": %d`, *nextProbeUnix)
		}
		extra := fmt.Sprintf(`{
			"upstream_billing_probe_enabled": true,
			"upstream_billing_probe": {"status": "ok", "next_probe_at": %q%s}
		}`, nextProbeAt, nextProbeUnixField)
		err := scanSingleRow(ctx, tx, `
			INSERT INTO accounts (name, platform, type, status, extra)
			VALUES ($1, 'openai', $2, 'active', $3::jsonb)
			RETURNING id
		`, []any{name, service.AccountTypeAPIKey, extra}, &id)
		require.NoError(t, err)
		return id
	}

	dueUnix := now.Add(-time.Second).Unix()
	notDueUnix := now.Add(time.Second).Unix()
	invalidID := insert("probe-invalid-calendar-date", "2026-99-99T12:00:00Z", nil)
	legacyDueID := insert("probe-legacy-due", "2026-07-14T11:59:58Z", nil)
	_ = insert("probe-legacy-not-due", "2026-07-14T12:00:02Z", nil)
	dueID := insert("probe-due", "2026-07-14T11:59:59Z", &dueUnix)
	_ = insert("probe-not-due", "2026-07-14T12:00:01Z", &notDueUnix)

	accounts, err := repo.ListDueUpstreamBillingProbeAccounts(ctx, now, 20)
	require.NoError(t, err)
	require.Len(t, accounts, 3)
	require.Equal(t, invalidID, accounts[0].ID)
	require.Equal(t, legacyDueID, accounts[1].ID)
	require.Equal(t, dueID, accounts[2].ID)
}
