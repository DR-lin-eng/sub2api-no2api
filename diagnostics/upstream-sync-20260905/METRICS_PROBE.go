package repository

import (
	"context"
	"database/sql/driver"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"os"
	"testing"
)

type syncMetricCapture struct{ value driver.Value }

func (c *syncMetricCapture) Match(value driver.Value) bool { c.value = value; return true }
func TestUpstreamSyncMetricsProbe(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	capture := &syncMetricCapture{}
	args := make([]driver.Value, 43)
	for i := range args {
		args[i] = sqlmock.AnyArg()
	}
	args[38] = capture
	mock.ExpectExec("INSERT INTO ops_system_metrics").WithArgs(args...).WillReturnResult(sqlmock.NewResult(1, 1))
	zero := 0
	if err := (&opsRepository{db: db}).InsertSystemMetrics(context.Background(), &service.OpsInsertSystemMetricsInput{DBConnActive: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	got := "NULL"
	if capture.value != nil {
		got = fmt.Sprint(capture.value)
	}
	fmt.Printf("DBConnActive(&0)=%s\n", got)
	if got != os.Getenv("SYNC_EXPECT") {
		t.Fatalf("got %s want %s", got, os.Getenv("SYNC_EXPECT"))
	}
}
