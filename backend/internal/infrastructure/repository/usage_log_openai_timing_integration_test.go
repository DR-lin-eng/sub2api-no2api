//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/openaitiming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenAITimingPersistence(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@timing.test"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-timing-" + uuid.NewString(), Name: "timing"})
	account := mustCreateAccount(t, client, &service.Account{Name: "timing-test"})
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			query string
			id    int64
		}{
			{"DELETE FROM usage_logs WHERE user_id=$1", user.ID},
			{"DELETE FROM api_keys WHERE id=$1", key.ID},
			{"DELETE FROM accounts WHERE id=$1", account.ID},
			{"DELETE FROM users WHERE id=$1", user.ID},
		} {
			_, err := integrationDB.ExecContext(ctx, cleanup.query, cleanup.id)
			require.NoError(t, err)
		}
	})
	var metrics openaitiming.Metrics
	require.NoError(t, json.Unmarshal([]byte(`{"first_sampled_message_ttft_ms":470,"engine_service_ttft_total_ms":690.87897,"engine_queue_max_ms":74,"total_turn_time_s":1.047620254}`), &metrics))
	for _, mode := range []string{"single", "batch", "best-effort", "no-result"} {
		t.Run(mode, func(t *testing.T) {
			first, duration := 1250, 1800
			log := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
				RequestID: uuid.NewString(), Model: "gpt-test", InputTokens: 11, OutputTokens: 7,
				FirstTokenMs: &first, DurationMs: &duration, OpenAITiming: &metrics, ActualCost: 0.1}
			var err error
			switch mode {
			case "single":
				_, err = repo.createSingle(ctx, integrationDB, log)
			case "batch":
				_, err = repo.Create(ctx, log)
			case "best-effort":
				err = repo.CreateBestEffort(ctx, log)
			case "no-result":
				err = execUsageLogInsertNoResult(ctx, integrationDB, prepareUsageLogInsert(log))
			}
			require.NoError(t, err)
			var id int64
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT id FROM usage_logs WHERE request_id=$1", log.RequestID).Scan(&id))
			stored, err := repo.GetByID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, 1250, *stored.FirstTokenMs)
			require.Equal(t, 1800, *stored.DurationMs)
			require.Equal(t, &metrics, stored.OpenAITiming)
			require.Equal(t, 11, stored.InputTokens)
			require.Equal(t, 7, stored.OutputTokens)
		})
	}
}
