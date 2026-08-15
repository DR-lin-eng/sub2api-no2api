package service

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/jackpal/gateway"
	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

type opsNetworkIOSnapshot struct {
	Interfaces       []string
	BytesReceived    uint64
	BytesTransmitted uint64
}

type opsNetworkIOReader func(context.Context) (opsNetworkIOSnapshot, error)

// readDefaultRouteNetworkIO intentionally samples only interfaces selected by
// the operating system's default IPv4/IPv6 routes. Falling back to every
// interface would double-count bridge/veth traffic and inflate public bandwidth.
func readDefaultRouteNetworkIO(ctx context.Context) (opsNetworkIOSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return opsNetworkIOSnapshot{}, err
	}

	interfaceNames, err := discoverDefaultRouteInterfaceNames()
	if err != nil {
		return opsNetworkIOSnapshot{}, err
	}
	counters, err := gopsutilnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return opsNetworkIOSnapshot{}, fmt.Errorf("read network counters: %w", err)
	}

	received, transmitted, found := aggregateDefaultRouteNetworkCounters(interfaceNames, counters)
	if !found {
		return opsNetworkIOSnapshot{}, fmt.Errorf("default route interface counters unavailable")
	}

	return opsNetworkIOSnapshot{
		Interfaces:       interfaceNames,
		BytesReceived:    received,
		BytesTransmitted: transmitted,
	}, nil
}

func aggregateDefaultRouteNetworkCounters(
	interfaceNames []string,
	counters []gopsutilnet.IOCountersStat,
) (received uint64, transmitted uint64, found bool) {
	wanted := make(map[string]struct{}, len(interfaceNames))
	for _, name := range interfaceNames {
		name = strings.TrimSpace(name)
		if name != "" {
			wanted[name] = struct{}{}
		}
	}
	for _, counter := range counters {
		if _, ok := wanted[counter.Name]; !ok {
			continue
		}
		received += counter.BytesRecv
		transmitted += counter.BytesSent
		found = true
	}
	return received, transmitted, found
}

func discoverDefaultRouteInterfaceNames() ([]string, error) {
	defaultIPs := make([]net.IP, 0, 2)
	if ip, err := gateway.DiscoverInterface(); err == nil && ip != nil {
		defaultIPs = append(defaultIPs, ip)
	}
	if ip, err := gateway.DiscoverInterfaceIPv6(); err == nil && ip != nil {
		defaultIPs = append(defaultIPs, ip)
	}
	if len(defaultIPs) == 0 {
		return nil, fmt.Errorf("default public route unavailable")
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	names := make(map[string]struct{}, len(defaultIPs))
	for _, iface := range interfaces {
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, address := range addresses {
			candidate := networkAddressIP(address)
			if candidate == nil {
				continue
			}
			for _, defaultIP := range defaultIPs {
				if defaultIP.Equal(candidate) {
					names[iface.Name] = struct{}{}
				}
			}
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("default public route interface not found")
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func networkAddressIP(address net.Addr) net.IP {
	if address == nil {
		return nil
	}
	raw := strings.TrimSpace(address.String())
	if raw == "" {
		return nil
	}
	if ip, _, err := net.ParseCIDR(raw); err == nil {
		return ip
	}
	return net.ParseIP(strings.Trim(raw, "[]"))
}

func (c *OpsMetricsCollector) collectDefaultRouteNetworkRates(
	ctx context.Context,
	sampleAt time.Time,
) (receiveBytesPerSecond *float64, transmitBytesPerSecond *float64, interfaces []string) {
	if c == nil {
		return nil, nil, nil
	}
	reader := c.networkIOReader
	if reader == nil {
		reader = readDefaultRouteNetworkIO
	}
	snapshot, err := reader(ctx)
	if err != nil || len(snapshot.Interfaces) == 0 {
		return nil, nil, nil
	}

	c.networkSampleMu.Lock()
	defer c.networkSampleMu.Unlock()

	interfaces = append([]string(nil), snapshot.Interfaces...)
	previous := c.lastNetworkSample
	previousAt := c.lastNetworkSampleAt
	c.lastNetworkSample = snapshot
	c.lastNetworkSampleAt = sampleAt

	if previousAt.IsZero() || !sameNetworkInterfaces(previous.Interfaces, snapshot.Interfaces) {
		return nil, nil, interfaces
	}
	if snapshot.BytesReceived < previous.BytesReceived || snapshot.BytesTransmitted < previous.BytesTransmitted {
		return nil, nil, interfaces
	}
	elapsed := sampleAt.Sub(previousAt)
	if elapsed <= 0 {
		return nil, nil, interfaces
	}

	receiveRate := roundTo1DP(float64(snapshot.BytesReceived-previous.BytesReceived) / elapsed.Seconds())
	transmitRate := roundTo1DP(float64(snapshot.BytesTransmitted-previous.BytesTransmitted) / elapsed.Seconds())
	return &receiveRate, &transmitRate, interfaces
}

func sameNetworkInterfaces(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
