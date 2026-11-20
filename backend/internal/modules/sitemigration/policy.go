package sitemigration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// These settings are rendered as text, or control local layout. The whitelist
// applies on both ends. Even an unset destination option can inherit authority
// through configuration fallback (e.g. a scheduled S3 backup or email sender).
var portableSettingKeys = []string{
	"site_name", "site_subtitle", "compact_home_enabled", "hide_ccs_import_button",
	"table_default_page_size", "table_page_size_options",
}

func isPortableSetting(key string) bool { return slices.Contains(portableSettingKeys, key) }

// Login-only wire schemas intentionally exclude audit/display metadata, even
// if a later application migration adds another column to one of these tables.
var loginColumns = map[string][]string{
	"users":                  strings.Fields(loginFields),
	"auth_identities":        strings.Fields("id user_id provider_type provider_key provider_subject verified_at issuer metadata"),
	"auth_identity_channels": strings.Fields("id identity_id provider_type provider_key channel channel_app_id channel_subject metadata"),
	"passkey_user_handles":   strings.Fields("user_id user_handle"),
	"passkey_credentials":    strings.Fields("id user_id credential_id credential_data"),
	"user_platform_quotas":   strings.Fields("id user_id platform daily_limit_usd weekly_limit_usd monthly_limit_usd daily_usage_usd weekly_usage_usd monthly_usage_usd daily_window_start weekly_window_start monthly_window_start deleted_at"),
}
var loginQuotaColumns = strings.Fields(quotaFields)

func selectedColumn(profile, table, column string) bool {
	if profile == "all" || profile == "clone" {
		return true
	}
	if profile != "login" && profile != "login-quota" {
		return false
	}
	if table == "user_platform_quotas" && profile != "login-quota" {
		return false
	}
	if profile == "login-quota" && table == "users" && slices.Contains(loginQuotaColumns, column) {
		return true
	}
	return slices.Contains(loginColumns[table], column)
}

// Each record for an excluded row contains only an operator-facing identifier.
// The two arms use separate projections so excluded secrets never enter the app.
func snapshotRows(ctx context.Context, tx *sql.Tx, t *table, profile string) (*sql.Rows, error) {
	where := ""
	var args []any
	if t.name == "settings" {
		where = "key=ANY($1)"
		args = append(args, pq.Array(portableSettingKeys))
	} else {
		userFilter := "role='user'"
		if profile != "all" {
			userFilter += " AND deleted_at IS NULL"
		}
		if profile == "login-quota" {
			userFilter += " AND frozen_balance=0"
		}
		users := "SELECT id FROM public.users WHERE " + userFilter
		switch t.name {
		case "users":
			where = userFilter
		case "auth_identity_channels":
			where = "identity_id IN (SELECT id FROM public.auth_identities WHERE user_id IN (" + users + "))"
		default:
			if slices.Contains(t.cols, "user_id") {
				where = "user_id IN (" + users + ")"
			}
		}
	}
	// For partial profiles, a login needs only OAuth identifiers and the
	// binding timestamp. Legacy WeChat authenticates against the stored
	// provider/channel subjects and app ID, not profile claims in metadata.
	projection := make([]string, 0, len(t.cols))
	for _, col := range t.cols {
		if profile != "all" && col == "metadata" && (t.name == "auth_identities" || t.name == "auth_identity_channels") {
			projection = append(projection, "'{}'::jsonb AS metadata")
		} else {
			projection = append(projection, pq.QuoteIdentifier(col))
		}
	}
	base := " FROM " + qt(t.name)
	if where == "" {
		return tx.QueryContext(ctx, "SELECT row_to_json(src),false FROM (SELECT "+strings.Join(projection, ",")+base+" ORDER BY "+columns(t.pk)+") src")
	}
	included := "SELECT row_to_json(src),false FROM (SELECT " + strings.Join(projection, ",") + base + " WHERE " + where + " ORDER BY " + columns(t.pk) + ") src"
	if profile != "all" {
		if profile == "login-quota" && t.name == "users" {
			included += " UNION ALL SELECT json_build_object('id',id,'email',email),true FROM public.users WHERE role='user' AND deleted_at IS NULL AND frozen_balance<>0"
		}
		return tx.QueryContext(ctx, included, args...)
	}
	id := "NULL"
	if slices.Contains(t.cols, "id") {
		id = "id"
	}
	fields := "'id'," + id
	if t.name == "users" {
		fields += ",'email',email"
	}
	if t.name == "settings" {
		// Unrecognized database setting names may themselves encode identifiers.
		fields += ",'key',CASE WHEN key=ANY($2) THEN key ELSE '' END"
		args = append(args, pq.Array(reportableSettingKeys))
	}
	excluded := "SELECT json_build_object(" + fields + "),true" + base + " WHERE (" + where + ") IS NOT TRUE"
	return tx.QueryContext(ctx, included+" UNION ALL "+excluded, args...)
}

// Unknown setting names are operator-supplied data, so even their names stay
// on the source. These static identifiers let operators identify a known
// destination-specific setting without disclosing its stored secret.
var reportableSettingKeys = strings.Fields("admin_api_keys admin_api_key legacy_admin_api_key_options permission_groups backup_s3_config backup_schedule backup_records frontend_url api_base_url smtp_host smtp_port smtp_password smtp_username smtp_from_email image_storage_config cloudflare_ingress_settings prompt_audit_config content_moderation_config totp_enabled passkey_enabled")

func redactSettingReason(key string) string {
	if slices.Contains(reportableSettingKeys, key) {
		return "站点专用设置由接收站管理员配置（" + key + "）"
	}
	return "站点专用设置由接收站管理员配置"
}

func recordPolicySkip(ctx context.Context, tx *sql.Tx, j *session, t *table, rec record) error {
	quotaUser := j.Profile == "login-quota" && t.name == "users"
	if j.Profile != "all" && !quotaUser {
		return ErrInvalid
	}
	if t.name != "settings" && t.name != "users" && t.name != "auth_identity_channels" && !slices.Contains(t.cols, "user_id") {
		return ErrInvalid
	}
	for k := range rec.Row {
		userEmail := t.name == "users" && k == "email"
		settingKey := t.name == "settings" && k == "key"
		if k != "id" && !userEmail && !settingKey {
			return ErrInvalid
		}
	}
	id := rec.Row["id"]
	if id == nil {
		return ErrInvalid
	}
	if !bytes.Equal(id, []byte("null")) {
		if _, err := strconv.ParseInt(string(id), 10, 64); err != nil {
			return ErrInvalid
		}
	}
	reason := "源记录属于受保护的管理账号"
	if t.name == "users" {
		reason = "管理账号和权限组由接收站管理员配置"
		if j.Profile == "login-quota" {
			reason = "用户尚有冻结余额，请在源站完成结算后重试"
		}
	}
	if t.name == "settings" {
		key := rec.Row.str("key")
		if isPortableSetting(key) {
			return ErrInvalid
		}
		reason = redactSettingReason(key)
	}
	im := importer{ctx: ctx, tx: tx, j: j}
	if t.name == "users" {
		email := rec.Row.str("email")
		if len(email) > 255 {
			return ErrInvalid
		}
		if email != "" {
			var dup bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM public.users WHERE deleted_at IS NULL AND "+emailExpr("email")+"="+emailExpr("$1::text")+")", email).Scan(&dup); err != nil {
				return err
			}
			if dup {
				reason = "相同邮箱或邮箱别名已存在，并且" + reason
			}
		}
	}
	return im.reject(t, rec.Row, reason)
}

func validatePolicyRow(profile string, t *table, r row) error {
	if t.name == "settings" && !isPortableSetting(r.str("key")) {
		return ErrInvalid
	}
	if t.name == "users" {
		// Require the exact JSON type. jsonb_populate_record also accepts
		// string-form booleans, which could bypass a pre-insert MFA check.
		if !bytes.Equal(r["totp_enabled"], []byte("true")) && !bytes.Equal(r["totp_enabled"], []byte("false")) {
			return ErrInvalid
		}
		if profile == "all" && r.str("role") != "user" {
			return ErrInvalid
		}
	}
	if profile != "all" && (t.name == "auth_identities" || t.name == "auth_identity_channels") {
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal(r["metadata"], &metadata); err != nil || metadata == nil || len(metadata) != 0 {
			return fmt.Errorf("extra login claims: %w", ErrInvalid)
		}
	}
	return nil
}
