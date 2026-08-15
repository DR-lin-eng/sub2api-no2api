package migrations

import (
	"strings"
	"testing"
)

func TestOpsNetworkBandwidthMigrationAddsDefaultRouteMetrics(t *testing.T) {
	content, err := FS.ReadFile("208_ops_network_bandwidth.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	for _, fragment := range []string{
		"network_receive_bytes_per_second DOUBLE PRECISION",
		"network_transmit_bytes_per_second DOUBLE PRECISION",
		"network_interfaces TEXT[]",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
