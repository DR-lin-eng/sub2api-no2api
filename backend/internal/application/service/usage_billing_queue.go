package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUsageBillingQueueUnavailable    = errors.New("usage billing queue is unavailable")
	ErrUsageBillingQueueFilterInvalid  = errors.New("usage billing queue filter is invalid")
	ErrUsageBillingQueueJobNotFound    = errors.New("usage billing queue job not found")
	ErrUsageBillingDeadLetterNotFound  = errors.New("usage billing dead letter not found")
	ErrUsageBillingAdminReasonRequired = errors.New("usage billing admin action reason is required")
	ErrUsageBillingDeadLetterInvalid   = errors.New("usage billing dead letter payload is invalid")
)

// UsageBillingRequestLoadSource supplies process-local gateway pressure to the
// load-aware queue controller without making the durable repository depend on
// an HTTP implementation.
type UsageBillingRequestLoadSource interface {
	InFlightRequests() int64
}

// UsageBillingWorkerLoadSource supplies the pre-persistence usage-worker
// backlog. The PostgreSQL queue remains the source of truth.
type UsageBillingWorkerLoadSource interface {
	UsageBillingWorkerBacklog() int64
}

// UsageBillingQueueLoadSink accepts optional runtime pressure sources after
// dependency construction. Implementations must tolerate nil sources.
type UsageBillingQueueLoadSink interface {
	SetUsageBillingRequestLoadSource(UsageBillingRequestLoadSource)
	SetUsageBillingWorkerLoadSource(UsageBillingWorkerLoadSource)
}

type UsageBillingQueueNodeStats struct {
	ConsumerMode           string            `json:"consumer_mode"`
	ConfiguredConsumers    int               `json:"configured_consumers"`
	EffectiveConsumers     int               `json:"effective_consumers"`
	ActiveConsumerBatches  int64             `json:"active_consumer_batches"`
	ClusterMaxConsumers    int               `json:"cluster_max_consumers"`
	RescueEnabled          bool              `json:"rescue_enabled"`
	CPUUsageEWMA           float64           `json:"cpu_usage_ewma"`
	CPUUsageKnown          bool              `json:"cpu_usage_known"`
	InFlightRequests       int64             `json:"in_flight_requests"`
	UsageWorkerBacklog     int64             `json:"usage_worker_backlog"`
	DBPoolWaitMilliseconds float64           `json:"db_pool_wait_milliseconds"`
	ReadyBacklog           bool              `json:"ready_backlog"`
	EnqueuedTotal          uint64            `json:"enqueued_total"`
	SettledTotal           uint64            `json:"settled_total"`
	CleanupTotal           uint64            `json:"cleanup_total"`
	RescuedUnsettledTotal  uint64            `json:"rescued_unsettled_total"`
	RescuedCleanupTotal    uint64            `json:"rescued_cleanup_total"`
	RetryTotal             uint64            `json:"retry_total"`
	ErrorTotal             uint64            `json:"error_total"`
	RetryClassTotals       map[string]uint64 `json:"retry_class_totals"`
	BatchDurationP95MS     float64           `json:"batch_duration_p95_ms"`
	ProcessedPerSecond     float64           `json:"processed_per_second"`
	CollectedAt            time.Time         `json:"collected_at"`
}

// UsageBillingQueueLoadSource is consumed by cluster heartbeats so each node's
// throughput and effective consumer count remain visible to operators.
type UsageBillingQueueLoadSource interface {
	UsageBillingQueueNodeStats() UsageBillingQueueNodeStats
}

type UsageBillingQueueSnapshot struct {
	UnsettledCount            int64                      `json:"unsettled_count"`
	CleanupPendingCount       int64                      `json:"cleanup_pending_count"`
	ReconcileRequiredCount    int64                      `json:"reconcile_required_count"`
	DeadLetterCount           int64                      `json:"dead_letter_count"`
	OldestUnsettledAgeSeconds float64                    `json:"oldest_unsettled_age_seconds"`
	OldestCleanupAgeSeconds   float64                    `json:"oldest_cleanup_age_seconds"`
	MaxSettlementAttempts     int                        `json:"max_settlement_attempts"`
	MaxCleanupAttempts        int                        `json:"max_cleanup_attempts"`
	SettlementAttemptsTotal   int64                      `json:"settlement_attempts_total"`
	CleanupAttemptsTotal      int64                      `json:"cleanup_attempts_total"`
	ErrorClassCounts          map[string]int64           `json:"error_class_counts"`
	Alerting                  bool                       `json:"alerting"`
	Node                      UsageBillingQueueNodeStats `json:"node"`
	CollectedAt               time.Time                  `json:"collected_at"`
}

type UsageBillingQueueJobFilter struct {
	State    string
	Query    string
	Page     int
	PageSize int
}

type UsageBillingQueueJob struct {
	ID                  int64      `json:"id"`
	RequestID           string     `json:"request_id"`
	APIKeyID            int64      `json:"api_key_id"`
	State               string     `json:"state"`
	Attempts            int        `json:"attempts"`
	CleanupAttempts     int        `json:"cleanup_attempts"`
	AvailableAt         time.Time  `json:"available_at"`
	LastError           string     `json:"last_error"`
	LastErrorClass      string     `json:"last_error_class"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	SettledAt           *time.Time `json:"settled_at"`
	LastAttemptAt       *time.Time `json:"last_attempt_at"`
	LastClaimedBy       string     `json:"last_claimed_by"`
	ReconcileRequiredAt *time.Time `json:"reconcile_required_at"`
}

type UsageBillingQueueJobList struct {
	Jobs     []UsageBillingQueueJob `json:"jobs"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type UsageBillingDeadLetter struct {
	ID             int64      `json:"id"`
	SourceJobID    int64      `json:"source_job_id"`
	RequestID      string     `json:"request_id"`
	APIKeyID       int64      `json:"api_key_id"`
	Attempts       int        `json:"attempts"`
	Reason         string     `json:"reason"`
	CreatedAt      time.Time  `json:"created_at"`
	FailedAt       time.Time  `json:"failed_at"`
	ReplayCount    int        `json:"replay_count"`
	LastReplayedAt *time.Time `json:"last_replayed_at"`
	LastReplayedBy *int64     `json:"last_replayed_by"`
	ReplayReason   string     `json:"replay_reason"`
}

type UsageBillingDeadLetterList struct {
	DeadLetters []UsageBillingDeadLetter `json:"dead_letters"`
	Total       int64                    `json:"total"`
	Page        int                      `json:"page"`
	PageSize    int                      `json:"page_size"`
}

type UsageBillingDeadLetterReplay struct {
	DeadLetterID int64
	OperatorID   int64
	Reason       string
}

type UsageBillingJobRetry struct {
	JobID      int64
	OperatorID int64
	Reason     string
}

// UsageBillingQueueAdmin is an optional capability exposed only when the
// durable PostgreSQL queue is enabled.
type UsageBillingQueueAdmin interface {
	GetUsageBillingQueueSnapshot(context.Context) (*UsageBillingQueueSnapshot, error)
	ListUsageBillingQueueJobs(context.Context, UsageBillingQueueJobFilter) (*UsageBillingQueueJobList, error)
	ListUsageBillingDeadLetters(context.Context, UsageBillingQueueJobFilter) (*UsageBillingDeadLetterList, error)
	RetryUsageBillingQueueJob(context.Context, UsageBillingJobRetry) error
	ReplayUsageBillingDeadLetter(context.Context, UsageBillingDeadLetterReplay) error
}

// UsageBillingQueueRuntimeCoordinator connects optional queue capabilities
// after Wire has constructed the gateway load sources, cluster heartbeat and
// Ops service. Keeping this glue in application avoids infrastructure imports
// in either consumer.
type UsageBillingQueueRuntimeCoordinator struct{}

func ProvideUsageBillingQueueRuntimeCoordinator(
	repo UsageBillingRepository,
	requestLoad *ClusterReleaseService,
	workerLoad *UsageRecordWorkerPool,
	cluster *ClusterService,
	ops *OpsService,
) *UsageBillingQueueRuntimeCoordinator {
	if sink, ok := repo.(UsageBillingQueueLoadSink); ok {
		sink.SetUsageBillingRequestLoadSource(requestLoad)
		sink.SetUsageBillingWorkerLoadSource(workerLoad)
	}
	if source, ok := repo.(UsageBillingQueueLoadSource); ok && cluster != nil {
		cluster.SetUsageBillingQueueLoadSource(source)
	}
	if admin, ok := repo.(UsageBillingQueueAdmin); ok && ops != nil {
		ops.SetUsageBillingQueueAdmin(admin)
	}
	return &UsageBillingQueueRuntimeCoordinator{}
}
