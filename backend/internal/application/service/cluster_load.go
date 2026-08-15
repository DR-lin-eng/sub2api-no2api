package service

import (
	"context"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type ClusterInstanceLoad struct {
	CPUUsagePercent        *float64                    `json:"cpu_usage_percent,omitempty"`
	MemoryUsedBytes        *int64                      `json:"memory_used_bytes,omitempty"`
	MemoryLimitBytes       *int64                      `json:"memory_limit_bytes,omitempty"`
	MemoryUsagePercent     *float64                    `json:"memory_usage_percent,omitempty"`
	InFlightRequests       int64                       `json:"in_flight_requests"`
	ActiveTasks            int                         `json:"active_tasks"`
	GoroutineCount         int                         `json:"goroutine_count"`
	DBConnectionsActive    int                         `json:"db_connections_active"`
	DBConnectionsIdle      int                         `json:"db_connections_idle"`
	DBConnectionsMax       int                         `json:"db_connections_max"`
	RedisConnectionsActive int                         `json:"redis_connections_active"`
	RedisConnectionsIdle   int                         `json:"redis_connections_idle"`
	RedisConnectionsMax    int                         `json:"redis_connections_max"`
	UsageBilling           *UsageBillingQueueNodeStats `json:"usage_billing,omitempty"`
	CollectedAt            time.Time                   `json:"collected_at"`
}

type ClusterConnectionStats struct {
	Active int
	Idle   int
	Max    int
}

type clusterConnectionStatsProvider interface {
	ClusterConnectionStats() ClusterConnectionStats
}

type clusterRequestLoadSource interface {
	InFlightRequests() int64
}

type ClusterLoadSampler interface {
	Sample(context.Context) ClusterInstanceLoad
}

type clusterLoadSampler = ClusterLoadSampler

type defaultClusterLoadSampler struct {
	mu sync.Mutex

	process *process.Process

	lastCgroupCPUUsageNanos uint64
	lastCgroupCPUSampleAt   time.Time
	lastSample              ClusterInstanceLoad
}

func newDefaultClusterLoadSampler() clusterLoadSampler {
	proc, _ := process.NewProcess(int32(os.Getpid()))
	return &defaultClusterLoadSampler{process: proc}
}

func NewClusterLoadSampler() ClusterLoadSampler {
	return newDefaultClusterLoadSampler()
}

func (s *defaultClusterLoadSampler) Sample(ctx context.Context) ClusterInstanceLoad {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastSample.CollectedAt.IsZero() && now.Sub(s.lastSample.CollectedAt) < time.Second {
		return s.lastSample
	}

	load := ClusterInstanceLoad{
		GoroutineCount: runtime.NumGoroutine(),
		CollectedAt:    now,
	}
	load.CPUUsagePercent = s.sampleCPU(ctx, now)
	load.MemoryUsedBytes, load.MemoryLimitBytes, load.MemoryUsagePercent = s.sampleMemory(ctx)
	s.lastSample = load
	return load
}

func (s *defaultClusterLoadSampler) sampleCPU(ctx context.Context, now time.Time) *float64 {
	if usageNanos, ok := readCgroupCPUUsageNanos(); ok {
		if s.lastCgroupCPUSampleAt.IsZero() || usageNanos < s.lastCgroupCPUUsageNanos {
			s.lastCgroupCPUUsageNanos = usageNanos
			s.lastCgroupCPUSampleAt = now
			return clusterFloat64(0)
		}
		elapsed := now.Sub(s.lastCgroupCPUSampleAt).Seconds()
		previous := s.lastCgroupCPUUsageNanos
		s.lastCgroupCPUUsageNanos = usageNanos
		s.lastCgroupCPUSampleAt = now
		if elapsed > 0 {
			cores := readCgroupCPULimitCores()
			if cores <= 0 {
				cores = float64(runtime.NumCPU())
			}
			usageSeconds := float64(usageNanos-previous) / float64(time.Second)
			return clusterFloat64(clusterRoundPercent(normalizeCgroupCPUPercent(usageSeconds, elapsed, cores)))
		}
	}

	if s.process == nil {
		return nil
	}
	value, err := s.process.PercentWithContext(ctx, 0)
	if err != nil {
		return nil
	}
	cores := runtime.NumCPU()
	if cores > 0 {
		value /= float64(cores)
	}
	return clusterFloat64(clusterRoundPercent(value))
}

func (s *defaultClusterLoadSampler) sampleMemory(ctx context.Context) (*int64, *int64, *float64) {
	if used, limit, ok := readCgroupMemoryBytes(); ok {
		usedValue := clusterUint64ToInt64(used)
		var limitValue *int64
		var percent *float64
		if limit > 0 {
			value := clusterUint64ToInt64(limit)
			limitValue = &value
			percent = clusterFloat64(clusterRoundPercent(float64(used) / float64(limit) * 100))
		}
		return &usedValue, limitValue, percent
	}

	if s.process == nil {
		return nil, nil, nil
	}
	info, err := s.process.MemoryInfoWithContext(ctx)
	if err != nil || info == nil {
		return nil, nil, nil
	}
	used := clusterUint64ToInt64(info.RSS)
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil || vm == nil || vm.Total == 0 {
		return &used, nil, nil
	}
	limit := clusterUint64ToInt64(vm.Total)
	percent := clusterRoundPercent(float64(info.RSS) / float64(vm.Total) * 100)
	return &used, &limit, clusterFloat64(percent)
}

func clusterRoundPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 100 {
		value = 100
	}
	return math.Round(value*10) / 10
}

func clusterFloat64(value float64) *float64 {
	return &value
}

func clusterUint64ToInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}
