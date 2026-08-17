package migrations

import (
	"strings"
	"testing"
)

func TestIPv6EgressPoolMigrationDefinesFailClosedAccountBindings(t *testing.T) {
	contents, err := FS.ReadFile("231_add_ipv6_egress_pools.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	checks := []string{
		"ADD COLUMN IF NOT EXISTS egress_mode",
		"DEFAULT 'inherit'",
		"CREATE TABLE IF NOT EXISTS ipv6_egress_pools",
		"CREATE TABLE IF NOT EXISTS account_egress_bindings",
		"account_id BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE",
		"source_ipv6 VARCHAR(45) NOT NULL UNIQUE",
		"pool_id BIGINT NOT NULL REFERENCES ipv6_egress_pools(id) ON DELETE RESTRICT",
		"CHECK (egress_mode IN ('inherit', 'direct', 'external_proxy', 'ipv6_pool'))",
	}
	for _, check := range checks {
		if !strings.Contains(sql, check) {
			t.Fatalf("migration missing %q", check)
		}
	}
}
