//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogReasoningTokensPersistence(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	user := mustCreateUser(t, client, &service.User{Email: "reasoning-persistence@test.invalid"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "reasoning-persistence-fixture", Name: "reasoning"})
	account := mustCreateAccount(t, client, &service.Account{Name: "reasoning"})
	repo := newUsageLogRepositoryWithSQL(client, tx)
	zero, count := 0, 1234
	for _, mode := range []string{"single", "batch", "best-effort", "no-result"} {
		for index, value := range []*int{nil, &zero, &count} {
			t.Run(fmt.Sprintf("%s/%d", mode, index), func(t *testing.T) {
				log := &service.UsageLog{
					UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
					RequestID: fmt.Sprintf("reasoning-%s-%d", mode, index), Model: "reasoning-model",
					InputTokens: 100, OutputTokens: 2000, ReasoningTokens: value,
				}
				prepared := prepareUsageLogInsert(log)
				switch mode {
				case "single":
					_, err := repo.createSingle(ctx, tx, log)
					require.NoError(t, err)
				case "batch":
					batchKey := usageLogBatchKey(log.RequestID, key.ID)
					query, args := buildUsageLogBatchInsertQuery([]string{batchKey}, map[string]usageLogInsertPrepared{batchKey: prepared})
					_, err := tx.ExecContext(ctx, query, args...)
					require.NoError(t, err)
				case "best-effort":
					query, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
					_, err := tx.ExecContext(ctx, query, args...)
					require.NoError(t, err)
				case "no-result":
					require.NoError(t, execUsageLogInsertNoResult(ctx, tx, prepared))
				}
				rows, err := tx.QueryContext(ctx, "SELECT "+usageLogSelectColumns+" FROM usage_logs WHERE request_id=$1 AND api_key_id=$2", log.RequestID, key.ID)
				require.NoError(t, err)
				require.True(t, rows.Next())
				got, err := scanUsageLog(rows)
				require.NoError(t, err)
				require.NoError(t, rows.Close())
				require.Equal(t, value, got.ReasoningTokens)
				require.Equal(t, 2000, got.OutputTokens)
			})
		}
	}
}
