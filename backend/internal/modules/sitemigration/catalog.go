package sitemigration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/lib/pq"
)

// This list is intentionally explicit. New application tables, neighbouring
// schemas, login sessions, cluster leases and deployment secrets are never
// published by catalog discovery.
const businessTables = `users auth_identities auth_identity_channels passkey_user_handles passkey_credentials user_platform_quotas
proxies accounts groups account_groups api_keys user_subscriptions user_allowed_groups user_group_rate_multipliers
user_attribute_definitions user_attribute_values user_avatars user_provider_default_grants channels channel_groups
channel_model_pricing channel_pricing_intervals channel_account_stats_pricing_rules channel_account_stats_model_pricing
channel_account_stats_pricing_intervals channel_monitor_request_templates channel_monitors channel_monitor_histories
channel_monitor_daily_rollups composite_model_routes account_quality_runs custom_model_request_templates custom_model_configs
error_passthrough_rules ipv6_egress_pools account_egress_bindings tls_fingerprint_profiles settings
subscription_plans payment_provider_instances payment_orders payment_audit_logs promo_codes promo_code_usages
redeem_codes redeem_code_usages user_affiliates user_affiliate_ledger announcements announcement_reads
act_campaigns act_checkin_records act_checkin_summaries act_participation_records chat_conversations chat_assets
chat_messages chat_message_assets chat_quick_replies scheduled_test_plans scheduled_test_results
usage_logs billing_usage_entries usage_billing_dedup usage_billing_dedup_archive usage_billing_dead_letters
usage_billing_admin_actions usage_dashboard_daily usage_dashboard_daily_users usage_dashboard_hourly
usage_dashboard_hourly_users usage_group_daily_rollups ops_account_usage_daily
batch_image_jobs batch_image_items batch_image_events video_balance_holds
ops_alert_rules ops_alert_events ops_error_logs ops_ingress_reject_aggregates ops_metrics_daily ops_metrics_hourly
ops_system_log_cleanup_audits ops_system_logs ops_system_metrics content_moderation_logs
prompt_audit_events audit_logs deleted_api_key_audits auth_identity_migration_reports orphan_allowed_groups_audit`
const userKeysProfile = "login-quota-keys"

func includesUserQuota(p string) bool { return p == "login-quota" || p == userKeysProfile }

const loginTables = "users auth_identities auth_identity_channels passkey_user_handles passkey_credentials"
const loginFields = "id email password_hash username status signup_source totp_enabled totp_enabled_at totp_secret_encrypted"
const quotaFields = "balance concurrency rpm_limit total_recharged"

type ref struct{ col, parent, key string }
type table struct {
	name     string
	cols, pk []string
	refs     []ref
	serial   bool
	nullable map[string]bool
}
type catalog struct {
	tables map[string]*table
	order  []string
	hash   string
}

func qt(s string) string { return "public." + pq.QuoteIdentifier(s) }
func columns(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = pq.QuoteIdentifier(c)
	}
	return strings.Join(q, ",")
}
func readCatalog(ctx context.Context, tx *sql.Tx, profile string) (catalog, error) {
	names := strings.Fields(businessTables)
	if profile == "clone" {
		names = append(names, strings.Fields(cloneExtraTables)...)
	} else if profile != "all" {
		names = strings.Fields(loginTables)
		if includesUserQuota(profile) {
			names = append(names, "user_platform_quotas")
			if profile == userKeysProfile {
				names = append(names, "api_keys")
			}
		}
	}
	c := catalog{tables: make(map[string]*table)}
	for _, n := range names {
		c.tables[n] = &table{name: n, nullable: map[string]bool{}}
	}
	rows, err := tx.QueryContext(ctx, `SELECT tab.relname,a.attname,format_type(a.atttypid,a.atttypmod),
        coalesce(a.attnum=ANY(idx.indkey),false),pg_get_serial_sequence(format('public.%I',tab.relname),a.attname) IS NOT NULL, NOT a.attnotnull
        FROM pg_class tab JOIN pg_attribute a ON a.attrelid=tab.oid
        LEFT JOIN pg_index idx ON idx.indrelid=tab.oid AND idx.indisprimary
        WHERE tab.relnamespace='public'::regnamespace AND tab.relname=ANY($1) AND tab.relkind='r'
        AND a.attnum>0 AND NOT a.attisdropped AND a.attgenerated='' ORDER BY tab.relname,a.attnum`, pq.Array(names))
	if err != nil {
		return c, err
	}
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	for rows.Next() {
		var name, col, typ string
		var pk, serial, nullable bool
		if err = rows.Scan(&name, &col, &typ, &pk, &serial, &nullable); err != nil {
			return c, err
		}
		if !selectedColumn(profile, name, col) {
			continue
		}
		t := c.tables[name]
		t.cols = append(t.cols, col)
		t.nullable[col] = nullable
		if pk {
			t.pk = append(t.pk, col)
		}
		if col == "id" {
			t.serial = serial
		}
		fmt.Fprintf(h, "%s/%s/%s/%t\n", name, col, typ, pk)
	}
	if err = rows.Err(); err != nil {
		return c, err
	}
	if err = rows.Close(); err != nil {
		return c, err
	}
	foreignKeys, err := tx.QueryContext(ctx, `SELECT t.relname,a.attname,p.relname,b.attname FROM pg_constraint f
      JOIN pg_class t ON t.oid=f.conrelid JOIN pg_class p ON p.oid=f.confrelid
      JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=f.conkey[1]
      JOIN pg_attribute b ON b.attrelid=p.oid AND b.attnum=f.confkey[1]
      WHERE f.contype='f' AND t.relnamespace='public'::regnamespace AND t.relname=ANY($1) ORDER BY 1,2`, pq.Array(names))
	if err != nil {
		return c, err
	}
	defer func() { _ = foreignKeys.Close() }()
	for foreignKeys.Next() {
		var n string
		var r ref
		if err = foreignKeys.Scan(&n, &r.col, &r.parent, &r.key); err != nil {
			return c, err
		}
		t := c.tables[n]
		if slices.Contains(t.cols, r.col) {
			t.refs = append(t.refs, r)
			fmt.Fprintln(h, n, r)
		}
	}
	if err = foreignKeys.Err(); err != nil {
		return c, err
	}
	for _, entry := range strings.Fields(extraRefs) {
		p := strings.Split(entry, ".")
		t := c.tables[p[0]]
		if t != nil && slices.Contains(t.cols, p[1]) {
			if !slices.ContainsFunc(t.refs, func(r ref) bool { return r.col == p[1] }) {
				key := "id"
				if len(p) == 4 {
					key = p[3]
				}
				t.refs = append(t.refs, ref{p[1], p[2], key})
			}
		}
	}
	// Deterministic topological order, excluding self references handled at row level.
	remaining := slices.Clone(names)
	sort.Strings(remaining)
	done := map[string]bool{}
	for len(c.order) < len(names) {
		before := len(c.order)
		for _, n := range remaining {
			t := c.tables[n]
			if done[n] {
				continue
			}
			if len(t.pk) == 0 {
				return c, fmt.Errorf("missing supported table or primary key: %s", n)
			}
			needsParent := func(parent string) bool { return parent != n && c.tables[parent] != nil && !done[parent] }
			if slices.ContainsFunc(t.refs, func(r ref) bool { return needsParent(r.parent) }) || slices.ContainsFunc(jsonParents[n], needsParent) {
				continue
			}
			c.order = append(c.order, n)
			done[n] = true
		}
		if len(c.order) == before {
			return c, errors.New("unsupported circular table relationship")
		}
	}
	c.hash = hex.EncodeToString(h.Sum(nil))
	return c, nil
}

// Includes ownership links that the production schema does not enforce with a FK.
const extraRefs = `accounts.proxy_fallback_origin_id.proxies usage_logs.channel_id.channels chat_messages.sender_id.users chat_messages.recalled_by.users
audit_logs.actor_user_id.users ops_account_usage_daily.account_id.accounts ops_account_usage_daily.last_usage_log_id.usage_logs
payment_orders.user_id.users payment_orders.refund_requested_by.users payment_orders.plan_id.subscription_plans payment_orders.subscription_group_id.groups payment_orders.provider_instance_id.payment_provider_instances
payment_audit_logs.order_id.payment_orders subscription_plans.group_id.groups channel_monitors.created_by.users
batch_image_jobs.user_id.users batch_image_jobs.api_key_id.api_keys batch_image_jobs.account_id.accounts batch_image_jobs.parent_batch_id.batch_image_jobs.batch_id
video_balance_holds.user_id.users video_balance_holds.api_key_id.api_keys video_balance_holds.account_id.accounts video_balance_holds.group_id.groups
deleted_api_key_audits.user_id.users deleted_api_key_audits.api_key_id.api_keys ops_error_logs.user_id.users ops_error_logs.account_id.accounts
ops_error_logs.api_key_id.api_keys ops_error_logs.group_id.groups ops_error_logs.resolved_by_user_id.users ops_error_logs.deleted_key_owner_user_id.users
ops_ingress_reject_aggregates.user_id.users ops_ingress_reject_aggregates.api_key_id.api_keys ops_system_logs.user_id.users ops_system_logs.account_id.accounts
ops_system_logs.api_key_id.api_keys ops_metrics_daily.group_id.groups ops_metrics_hourly.group_id.groups ops_system_metrics.group_id.groups
ops_system_log_cleanup_audits.operator_id.users ops_alert_events.rule_id.ops_alert_rules usage_billing_jobs.api_key_id.api_keys usage_billing_dedup.api_key_id.api_keys
usage_billing_dedup_archive.api_key_id.api_keys usage_billing_dead_letters.api_key_id.api_keys usage_billing_admin_actions.operator_id.users
usage_billing_admin_actions.source_job_id.usage_billing_jobs usage_billing_dead_letters.last_replayed_by.users usage_billing_admin_actions.api_key_id.api_keys usage_billing_admin_actions.source_dead_letter_id.usage_billing_dead_letters
usage_dashboard_daily_users.user_id.users usage_dashboard_hourly_users.user_id.users usage_group_daily_rollups.group_id.groups
orphan_allowed_groups_audit.user_id.users orphan_allowed_groups_audit.group_id.groups auth_identity_migration_reports.resolved_by_user_id.users`
