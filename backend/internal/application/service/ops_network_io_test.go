package service

import (
	"context"
	"testing"
	"time"

	gopsutilnet "github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/require"
)

func TestAggregateDefaultRouteNetworkCountersExcludesOtherInterfaces(t *testing.T) {
	received, transmitted, found := aggregateDefaultRouteNetworkCounters(
		[]string{"eth0", "wan6"},
		[]gopsutilnet.IOCountersStat{
			{Name: "lo", BytesRecv: 9000, BytesSent: 9000},
			{Name: "docker0", BytesRecv: 8000, BytesSent: 7000},
			{Name: "eth0", BytesRecv: 1200, BytesSent: 300},
			{Name: "wan6", BytesRecv: 400, BytesSent: 100},
		},
	)

	require.True(t, found)
	require.Equal(t, uint64(1600), received)
	require.Equal(t, uint64(400), transmitted)
}

func TestCollectDefaultRouteNetworkRatesUsesConsecutiveSamples(t *testing.T) {
	snapshots := []opsNetworkIOSnapshot{
		{Interfaces: []string{"eth0"}, BytesReceived: 1000, BytesTransmitted: 2000},
		{Interfaces: []string{"eth0"}, BytesReceived: 1600, BytesTransmitted: 2300},
		{Interfaces: []string{"wan0"}, BytesReceived: 5000, BytesTransmitted: 6000},
		{Interfaces: []string{"wan0"}, BytesReceived: 6200, BytesTransmitted: 6600},
		{Interfaces: []string{"wan0"}, BytesReceived: 100, BytesTransmitted: 200},
	}
	index := 0
	collector := &OpsMetricsCollector{
		networkIOReader: func(context.Context) (opsNetworkIOSnapshot, error) {
			value := snapshots[index]
			index++
			return value, nil
		},
	}
	start := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	receive, transmit, interfaces := collector.collectDefaultRouteNetworkRates(context.Background(), start)
	require.Nil(t, receive)
	require.Nil(t, transmit)
	require.Equal(t, []string{"eth0"}, interfaces)

	receive, transmit, _ = collector.collectDefaultRouteNetworkRates(context.Background(), start.Add(time.Minute))
	require.NotNil(t, receive)
	require.NotNil(t, transmit)
	require.InDelta(t, 10, *receive, 0.001)
	require.InDelta(t, 5, *transmit, 0.001)

	receive, transmit, interfaces = collector.collectDefaultRouteNetworkRates(context.Background(), start.Add(2*time.Minute))
	require.Nil(t, receive)
	require.Nil(t, transmit)
	require.Equal(t, []string{"wan0"}, interfaces)

	receive, transmit, _ = collector.collectDefaultRouteNetworkRates(context.Background(), start.Add(3*time.Minute))
	require.NotNil(t, receive)
	require.NotNil(t, transmit)
	require.InDelta(t, 20, *receive, 0.001)
	require.InDelta(t, 10, *transmit, 0.001)

	receive, transmit, _ = collector.collectDefaultRouteNetworkRates(context.Background(), start.Add(4*time.Minute))
	require.Nil(t, receive)
	require.Nil(t, transmit)
}
