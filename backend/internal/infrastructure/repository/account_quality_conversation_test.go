package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type qualityDetailsArg struct{}

func (qualityDetailsArg) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var details service.AccountQualityProbeDetails
	return json.Unmarshal([]byte(text), &details) == nil && details.Stage1 != nil && details.Stage1.Answer == "21" && details.Stage1.ConversationID == "conv_1"
}
func TestQualityConversationRepositoryRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewAccountQualityArtifactRepository(db)
	now := time.Now().UTC()
	id := "719a94e5-4f93-4ab8-a495-4b430be08a10"
	run := &service.AccountQualityRun{ID: id, AccountID: 1, Status: "ready", StartedAt: now, FinishedAt: now, Details: service.AccountQualityProbeDetails{Stage1: &service.AccountQualityStageDetail{Status: "passed", Answer: "21", ConversationID: "conv_1"}}}
	mock.ExpectExec("INSERT INTO account_quality_runs").WithArgs(id, int64(1), "", "", "ready", "", float64(0), now, now, int64(0), "", []byte(nil), []byte(nil), "", qualityDetailsArg{}).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM account_quality_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM account_quality_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, repo.Save(context.Background(), run))
	details, _ := json.Marshal(run.Details)
	mock.ExpectQuery("SELECT id::text, account_id").WithArgs(now, 200).WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "model", "effort", "status", "label", "confidence", "started_at", "finished_at", "latency_ms", "model_version", "error_message", "probe_details", "has_preview"}).AddRow(id, 1, "", "", "ready", "", 0, now, now, 0, "", "", details, false))
	rows, err := repo.ListPublic(context.Background(), now, 200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, run.Details, rows[0].Details)
	require.False(t, rows[0].HasPreview)
	require.NoError(t, mock.ExpectationsWereMet())
}
