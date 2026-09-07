//go:build unit

package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/openaitiming"
	"github.com/stretchr/testify/require"
)

func newSessionIDUsageLog(sessionID *string) *service.UsageLog {
	return &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-session-id",
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.0,
		ActualCost:   1.0,
		SessionID:    sessionID,
		CreatedAt:    time.Now().UTC(),
	}
}

func usageLogInsertColumnIndex(t *testing.T, column string) int {
	t.Helper()
	columns := strings.Split(usageLogSelectColumns, ",")
	require.Equal(t, "id", strings.TrimSpace(columns[0]))
	require.Len(t, usageLogInsertArgTypes, len(columns)-1, "SELECT includes the generated id before INSERT columns")
	for i, name := range columns[1:] {
		if strings.TrimSpace(name) == column {
			return i
		}
	}
	t.Fatalf("missing INSERT column %q", column)
	return -1
}

// Session ID wiring follows column order, independent of later appended fields.
func TestPrepareUsageLogInsert_SessionIDArgWiring(t *testing.T) {
	index := usageLogInsertColumnIndex(t, "session_id")
	sessionID := "sess-persisted-123"
	prepared := prepareUsageLogInsert(newSessionIDUsageLog(&sessionID))

	require.Len(t, prepared.args, len(usageLogInsertArgTypes),
		"prepared args must match the arg-type table length")

	sessionArg := prepared.args[index]
	ns, ok := sessionArg.(sql.NullString)
	require.True(t, ok, "session_id arg should be a sql.NullString, got %T", sessionArg)
	require.True(t, ns.Valid)
	require.Equal(t, sessionID, ns.String)

	require.Equal(t, "text", usageLogInsertArgTypes[index],
		"session_id arg type must be text")
}

// TestPrepareUsageLogInsert_SessionIDNullWhenAbsent proves an absent session id is
// persisted as SQL NULL rather than an empty string.
func TestPrepareUsageLogInsert_SessionIDNullWhenAbsent(t *testing.T) {
	index := usageLogInsertColumnIndex(t, "session_id")
	prepared := prepareUsageLogInsert(newSessionIDUsageLog(nil))
	sessionArg := prepared.args[index]
	ns, ok := sessionArg.(sql.NullString)
	require.True(t, ok, "session_id arg should be a sql.NullString, got %T", sessionArg)
	require.False(t, ns.Valid, "absent session id must be NULL, not empty string")

	empty := ""
	preparedEmpty := prepareUsageLogInsert(newSessionIDUsageLog(&empty))
	nsEmpty := preparedEmpty.args[index].(sql.NullString)
	require.False(t, nsEmpty.Valid, "empty session id must also be NULL")
}

func TestPrepareUsageLogInsert_OpenAITimingArgWiring(t *testing.T) {
	index := usageLogInsertColumnIndex(t, "openai_timing")
	require.Equal(t, "jsonb", usageLogInsertArgTypes[index])
	log := newSessionIDUsageLog(nil)
	require.Nil(t, prepareUsageLogInsert(log).args[index])
	engineTTFT, total := 690.87897, 1.047620254
	log.OpenAITiming = &openaitiming.Metrics{EngineServiceTTFTTotalMs: &engineTTFT, TotalTurnTimeSeconds: &total}
	prepared := prepareUsageLogInsert(log)
	payload, ok := prepared.args[index].(string)
	require.True(t, ok)
	var stored openaitiming.Metrics
	require.NoError(t, json.Unmarshal([]byte(payload), &stored))
	require.Equal(t, log.OpenAITiming, &stored)
	require.Equal(t, log.CreatedAt, prepared.args[usageLogInsertColumnIndex(t, "created_at")])
}

// TestUsageLogInsertQueries_IncludeSessionID guards that every generated INSERT path
// and the SELECT column list reference session_id.
func TestUsageLogInsertQueries_IncludeSessionID(t *testing.T) {
	require.Contains(t, usageLogSelectColumns, "session_id",
		"SELECT column list must include session_id")

	sessionID := "sess-in-query"
	log := newSessionIDUsageLog(&sessionID)
	prepared := prepareUsageLogInsert(log)
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)

	batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{key},
		map[string]usageLogInsertPrepared{key: prepared})
	require.Contains(t, batchQuery, "session_id")
	// Two column references (INSERT column list + SELECT ... FROM input) plus the CTE def.
	require.GreaterOrEqual(t, strings.Count(batchQuery, "session_id"), 3)
	require.Len(t, batchArgs, len(prepared.args)+1,
		"batch args include the synthetic input_index before usage-log values")

	bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
	require.Contains(t, bestEffortQuery, "session_id")
	require.Len(t, bestEffortArgs, len(prepared.args))
}
