package service

import (
	"testing"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/require"
)

const testMemoryMiB uint64 = 1024 * 1024

func TestResolveMemoryStatsUnlimitedCgroupFallsBackToHost(t *testing.T) {
	host := &mem.VirtualMemoryStat{Used: 16 * 1024 * testMemoryMiB, Total: 24 * 1024 * testMemoryMiB, UsedPercent: 66.7}
	used, total, pct := resolveMemoryStats(64*testMemoryMiB, 0, true, host)
	require.Equal(t, int64(16*1024), *used)
	require.Equal(t, int64(24*1024), *total)
	require.InDelta(t, 66.7, *pct, 0.05)
}

func TestResolveMemoryStatsLimitedCgroupUsesContainerTuple(t *testing.T) {
	host := &mem.VirtualMemoryStat{Used: 16 * 1024 * testMemoryMiB, Total: 24 * 1024 * testMemoryMiB, UsedPercent: 66.7}
	used, total, pct := resolveMemoryStats(512*testMemoryMiB, 2*1024*testMemoryMiB, true, host)
	require.Equal(t, int64(512), *used)
	require.Equal(t, int64(2048), *total)
	require.InDelta(t, 25.0, *pct, 0.05)
}

func TestResolveMemoryStatsNoDataReturnsNil(t *testing.T) {
	used, total, pct := resolveMemoryStats(0, 0, false, nil)
	require.Nil(t, used)
	require.Nil(t, total)
	require.Nil(t, pct)
}
