package sitemigration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLoginAndDeploymentPolicy(t *testing.T) {
	for _, key := range []string{"backup_schedule", "backup_s3_config", "smtp_host", "frontend_url", "prompt_audit_config", "admin_api_keys", "oidc_connect_client_secret", "future_endpoint"} {
		if isPortableSetting(key) || validatePolicyRow("all", &table{name: "settings"}, row{"key": json.RawMessage(`"` + key + `"`)}) == nil {
			t.Fatalf("unsafe setting accepted: %s", key)
		}
	}
	if !selectedColumn("login", "users", "totp_secret_encrypted") || selectedColumn("login", "users", "balance") || !selectedColumn("login-quota", "users", "balance") {
		t.Fatal("incorrect credential/quota projection")
	}
	if selectedColumn("login", "auth_identities", "future_secret") || selectedColumn("login", "passkey_credentials", "last_used_at") {
		t.Fatal("unexpected credential columns")
	}
	if validatePolicyRow("login", &table{name: "auth_identities"}, row{"metadata": json.RawMessage(`{"invitation_code":"fixture"}`)}) == nil {
		t.Fatal("unexpected OAuth claims")
	}
	if validatePolicyRow("login", &table{name: "users"}, row{"totp_enabled": json.RawMessage(`"true"`)}) == nil {
		t.Fatal("ambiguous MFA flag accepted")
	}
}

func TestPortableCredentialRewrap(t *testing.T) {
	source := bytes.Repeat([]byte{0x04}, 32)
	target := bytes.Repeat([]byte{0x09}, 32)
	stored, err := changeSecret("test-account-totp", source, false)
	if err != nil {
		t.Fatal(err)
	}
	r := row{}
	r.set("totp_secret_encrypted", stored)
	if err = portableSecrets("users", r, source, true); err != nil {
		t.Fatal(err)
	}
	if err = portableSecrets("users", r, target, false); err != nil {
		t.Fatal(err)
	}
	if got, e := changeSecret(r.str("totp_secret_encrypted"), target, true); e != nil || got != "test-account-totp" {
		t.Fatal("destination secret did not round-trip")
	}
	if _, e := changeSecret(r.str("totp_secret_encrypted"), source, true); e == nil {
		t.Fatal("destination still uses the source key")
	}
}

// Run against a disposable, initialized Sub2API database. All inserts are
// rolled back. The test exercises the rows leaving PostgreSQL before encryption.
func TestSnapshotProjectionPostgres(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_POLICY_TEST_DSN")
	if dsn == "" {
		t.Skip("set SITE_MIGRATION_POLICY_TEST_DSN to a disposable application database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	token := uuid.NewString()
	addUser := func(role, balance string) int64 {
		var id int64
		err := tx.QueryRowContext(ctx, "INSERT INTO public.users(email,password_hash,role,frozen_balance) VALUES ($1,$2,$3,$4) RETURNING id", role+balance+"."+token+"@example.test", "fixture-password-"+role, role, balance).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	admin := addUser("admin", "0")
	normal := addUser("user", "0")
	frozen := addUser("user", "1")
	_, err = tx.ExecContext(ctx, "INSERT INTO public.settings(key,value) VALUES ('backup_s3_config',$1) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value", "private-object-store-"+token)
	if err != nil {
		t.Fatal(err)
	}
	var identity int64
	err = tx.QueryRowContext(ctx, `INSERT INTO public.auth_identities(user_id,provider_type,provider_key,provider_subject,metadata) VALUES($1,'linuxdo','linuxdo',$2,'{"invitation_code":"private profile field"}') RETURNING id`, normal, token).Scan(&identity)
	if err != nil {
		t.Fatal(err)
	}
	extract := func(profile, name string, match func(row) bool) (row, bool) {
		c, e := readCatalog(ctx, tx, profile)
		if e != nil {
			t.Fatal(e)
		}
		rows, e := snapshotRows(ctx, tx, c.tables[name], profile)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var b []byte
			var r row
			var omitted bool
			if e = rows.Scan(&b, &omitted); e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(b, &r); e != nil {
				t.Fatal(e)
			}
			if match(r) {
				return r, omitted
			}
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		t.Fatalf("missing test fixture in %s", name)
		return nil, false
	}
	byID := func(id int64) func(row) bool {
		return func(r row) bool { var got int64; return json.Unmarshal(r["id"], &got) == nil && got == id }
	}
	r, omit := extract("all", "users", byID(admin))
	if !omit || len(r) != 2 || r["password_hash"] != nil {
		t.Fatal("source exposed administrative credentials")
	}
	r, omit = extract("all", "settings", func(r row) bool { return r.str("key") == "backup_s3_config" })
	if !omit || r["value"] != nil {
		t.Fatal("source exposed outbound configuration")
	}
	r, omit = extract("login", "auth_identities", byID(identity))
	if omit || !bytes.Equal(r["metadata"], []byte("{}")) || r["created_at"] != nil {
		t.Fatal("partial snapshot exposed non-login claims")
	}
	r, omit = extract("login-quota", "users", byID(frozen))
	if !omit || r["balance"] != nil {
		t.Fatal("unsettled principal exported")
	}

	// WeChat also resolves the legacy provider key and bytea identities use
	// binary equality even if two database clients choose different encodings.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE migration_rows(n bigint,t text,data jsonb) ON COMMIT DROP`)
	if err != nil {
		t.Fatal(err)
	}
	im := importer{ctx: ctx, tx: tx}
	if err = im.buildEmailIndex(); err != nil {
		t.Fatal(err)
	}
	subject := "wechat-" + token
	_, err = tx.ExecContext(ctx, "INSERT INTO public.auth_identities(user_id,provider_type,provider_key,provider_subject) VALUES ($1,'wechat','wechat',$2)", normal, subject)
	if err != nil {
		t.Fatal(err)
	}
	sourceUser := row{"id": json.RawMessage("1122334411")}
	sourceUser.set("email", token+".other@example.test")
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_rows(t,data) VALUES('auth_identities',jsonb_build_object('user_id',1122334411,'provider_type','wechat','provider_key','wechat-main','provider_subject',$1::text))`, subject)
	if err != nil {
		t.Fatal(err)
	}
	reason, err := im.userConflict(sourceUser)
	if err != nil || reason == "" {
		t.Fatalf("legacy WeChat collision missing: %v", err)
	}
	_, err = tx.ExecContext(ctx, "TRUNCATE migration_rows")
	if err != nil {
		t.Fatal(err)
	}
	handle := []byte("handle-" + token)
	_, err = tx.ExecContext(ctx, "INSERT INTO public.passkey_user_handles(user_id,user_handle) VALUES ($1,$2)", normal, handle)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_rows(t,data) VALUES('passkey_user_handles',jsonb_build_object('user_id',1122334411,'user_handle',$1::text))`, "\\x"+hex.EncodeToString(handle))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, "SET LOCAL bytea_output='escape'")
	if err != nil {
		t.Fatal(err)
	}
	reason, err = im.userConflict(sourceUser)
	if err != nil || reason == "" {
		t.Fatalf("binary Passkey collision missing: %v", err)
	}
}

func TestMigrationReportSettingNames(t *testing.T) {
	for _, bad := range []string{"=SUM(1,2)", "unknown/result", "upperCASE", "1prefix"} {
		if redactSettingReason(bad) != "站点专用设置由接收站管理员配置" {
			t.Fatal("untrusted setting label included in report")
		}
	}
}
