package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func TestQueryOpsUsageCoreSharesTotalsTrendAndHistogram(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"bucket", "success_count", "token_consumed", "current_success", "current_tokens",
		"lt100", "100_200", "200_500", "500_1000", "1000_2000", "gte2000",
	}).
		AddRow(start, int64(4), int64(400), int64(0), int64(0), int64(1), int64(1), int64(1), int64(1), int64(0), int64(0)).
		AddRow(start.Add(time.Minute), int64(6), int64(900), int64(6), int64(900), int64(0), int64(1), int64(1), int64(1), int64(1), int64(2))
	mock.ExpectQuery(`current_success`).WithArgs(start, end).WillReturnRows(rows)

	result, err := repo.queryOpsUsageCore(context.Background(), &service.OpsDashboardFilter{}, start, end, 60, true)
	if err != nil {
		t.Fatalf("queryOpsUsageCore() error = %v", err)
	}
	if result.successCount != 10 || result.tokenConsumed != 1300 || result.currentSuccess != 6 || result.currentTokens != 900 {
		t.Fatalf("unexpected totals: %+v", result)
	}
	if len(result.points) != 2 || result.points[1].TPS != 15 {
		t.Fatalf("unexpected points: %+v", result.points)
	}
	if result.histogram == nil || result.histogram.TotalRequests != 10 || result.histogram.Buckets[5].Count != 2 {
		t.Fatalf("unexpected histogram: %+v", result.histogram)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueryOpsErrorCoreSharesTrendTotalsAndDistribution(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	rows := sqlmock.NewRows([]string{
		"row_kind", "bucket", "grouped_status", "error_total", "business_limited", "error_sla",
		"upstream_excl", "upstream_429", "upstream_529", "current_errors",
	}).
		AddRow(0, start, nil, int64(7), int64(2), int64(5), int64(1), int64(3), int64(1), int64(4)).
		AddRow(1, nil, int64(429), int64(4), int64(1), int64(3), int64(0), int64(3), int64(0), int64(3)).
		AddRow(1, nil, int64(500), int64(3), int64(1), int64(2), int64(1), int64(0), int64(0), int64(1)).
		AddRow(1, nil, int64(200), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0))
	mock.ExpectQuery(`GROUP BY GROUPING SETS`).WithArgs(start, end).WillReturnRows(rows)

	result, err := repo.queryOpsErrorCore(context.Background(), &service.OpsDashboardFilter{}, start, end, 60, true)
	if err != nil {
		t.Fatalf("queryOpsErrorCore() error = %v", err)
	}
	if result.errorCountTotal != 7 || result.businessLimitedCount != 2 || result.currentErrors != 4 {
		t.Fatalf("unexpected totals: %+v", result)
	}
	if len(result.trend.Points) != 1 || result.trend.Points[0].Upstream429Count != 3 {
		t.Fatalf("unexpected trend: %+v", result.trend)
	}
	if result.distribution == nil || result.distribution.Total != 7 || len(result.distribution.Items) != 2 || result.distribution.Items[0].StatusCode != 429 {
		t.Fatalf("unexpected distribution: %+v", result.distribution)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeOpsErrorDistributionPreservesTop20Total(t *testing.T) {
	distribution := &service.OpsErrorDistributionResponse{
		Items: make([]*service.OpsErrorDistributionItem, 0, 21),
	}
	for index := 1; index <= 21; index++ {
		distribution.Items = append(distribution.Items, &service.OpsErrorDistributionItem{
			StatusCode: 400 + index,
			Total:      int64(index),
		})
	}

	finalizeOpsErrorDistribution(distribution)
	if len(distribution.Items) != 20 {
		t.Fatalf("items = %d, want 20", len(distribution.Items))
	}
	if distribution.Total != 230 {
		t.Fatalf("total = %d, want 230", distribution.Total)
	}
}
