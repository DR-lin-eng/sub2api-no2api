//go:build integration

package sitemigration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// All data is fictional and rolled back. Use a disposable, migrated PostgreSQL
// site. Exercise both source projection and destination merge with different IDs.
func TestOrdinaryUserKeysQuotaAndAdminBoundary(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires a migrated test site")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer func() { _ = db.Close() }()
	ctx := t.Context()
	tx, e := db.BeginTx(ctx, nil)
	require.NoError(t, e)
	defer func() { _ = tx.Rollback() }()
	prefix := uuid.NewString()
	email := prefix + "@example.test"
	adminEmail := "staff-" + email
	hash, e := bcrypt.GenerateFromPassword([]byte("test-original-password"), bcrypt.MinCost)
	require.NoError(t, e)
	var user, staff, g int64
	require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash,balance) VALUES($1,$2,123.12345678) RETURNING id", email, string(hash)).Scan(&user))
	require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash,role) VALUES($1,$2,'admin') RETURNING id", adminEmail, "admin-secret-"+prefix).Scan(&staff))
	require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO groups(name) VALUES($1) RETURNING id", prefix).Scan(&g))
	key := "sk-group-" + prefix
	plain := "sk-free-" + prefix
	adminKey := "sk-admin-" + prefix
	_, e = tx.ExecContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id,group_bindings,quota,quota_used,ip_whitelist,rate_limit_5h,usage_5h,window_5h_start,expires_at) VALUES($1,$2,'grouped',$3,jsonb_build_array(jsonb_build_object('group_id',$3::bigint)),100,17.12345678,'["127.0.0.1"]',20,3,now(),now()+interval '7 days')`, user, key, g)
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'free'),($3,$4,'staff')", user, plain, staff, adminKey)
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "INSERT INTO user_platform_quotas(user_id,platform,daily_limit_usd,daily_usage_usd) VALUES($1,'openai',12,3)", user)
	require.NoError(t, e)
	store := Store{DataDir: t.TempDir()}
	var snapshot bytes.Buffer
	require.NoError(t, store.exportSnapshot(ctx, tx, userKeysProfile, &snapshot))
	require.Contains(t, snapshot.String(), key)
	require.NotContains(t, snapshot.String(), adminKey)
	require.NotContains(t, snapshot.String(), adminEmail)
	require.NotContains(t, snapshot.String(), "admin-secret-"+prefix)
	var limited bytes.Buffer
	require.NoError(t, store.exportSnapshot(ctx, tx, "login-quota", &limited))
	require.NotContains(t, limited.String(), plain)
	_, e = tx.ExecContext(ctx, "DELETE FROM user_platform_quotas WHERE user_id=$1", user)
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "DELETE FROM api_keys WHERE user_id=$1", user)
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "DELETE FROM users WHERE id=$1", user)
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "UPDATE site_migration_identity SET site=gen_random_uuid()")
	require.NoError(t, e)
	j := &session{State: State{ID: uuid.NewString(), Profile: userKeysProfile}}
	j.aead, e = makeCipher([]byte("synthetic-only"), []byte(j.ID), "user transfer")
	require.NoError(t, e)
	_, e = tx.ExecContext(ctx, "INSERT INTO site_migration_jobs(id,owner,mode,profile,local_public,wrapped_private) VALUES($1,'unit','import',$2,'','')", j.ID, j.Profile)
	require.NoError(t, e)
	require.NoError(t, importSnapshot(ctx, tx, j, bytes.NewReader(snapshot.Bytes())))
	var uid int64
	var password, balance string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT id,password_hash,balance::text FROM users WHERE email=$1", email).Scan(&uid, &password, &balance))
	require.NotEqual(t, user, uid)
	require.Equal(t, "123.12345678", balance)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(password), []byte("test-original-password")))
	var remaining, status, bindings, ips string
	var group sql.NullInt64
	var valid bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT (quota-quota_used)::text,status,group_id,group_bindings::text,ip_whitelist::text,expires_at>now() AND rate_limit_5h=20 AND usage_5h=3 AND window_5h_start IS NOT NULL FROM api_keys WHERE user_id=$1 AND key=$2`, uid, key).Scan(&remaining, &status, &group, &bindings, &ips, &valid))
	require.Equal(t, "82.87654322", remaining)
	require.Equal(t, "inactive", status)
	require.False(t, group.Valid)
	require.Equal(t, "[]", bindings)
	require.Contains(t, ips, "127.0.0.1")
	require.True(t, valid)
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE user_id=$1 AND key=$2", uid, plain).Scan(&status))
	require.Equal(t, "inactive", status)
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT daily_limit_usd-daily_usage_usd=9 FROM user_platform_quotas WHERE user_id=$1", uid).Scan(&valid))
	require.True(t, valid)
	for i := 0; i < j.Errors; i++ {
		b, err := getBlock(ctx, tx, j.ID, 1, i)
		require.NoError(t, err)
		line, _, err := j.plain(1, i, b)
		require.NoError(t, err)
		_, err = csv.NewReader(bytes.NewReader(line)).Read()
		require.NoError(t, err)
		require.NotContains(t, string(line), key)
	}
}

func TestCloneSourceAndDestinationExcludePrivilegedPrincipals(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires a migrated test site")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	tx, e := db.BeginTx(ctx, nil)
	require.NoError(t, e)
	defer func() { _ = tx.Rollback() }()
	tag := uuid.NewString()
	for _, role := range []string{"admin", "support", "user"} {
		var id int64
		require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash,role) VALUES($1,$2,$3) RETURNING id", role+tag+"@example.test", role+"-hash-"+tag, role).Scan(&id))
		_, e = tx.ExecContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'fixture')", id, role+"-key-"+tag)
		require.NoError(t, e)
		_, e = tx.ExecContext(ctx, "INSERT INTO auth_identities(user_id,provider_type,provider_key,provider_subject) VALUES($1,'email','email',$2)", id, role+"-identity-"+tag)
		require.NoError(t, e)
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO settings(key,value,updated_at) VALUES('admin_api_keys',$1,now()) ON CONFLICT(key) DO UPDATE SET value=excluded.value", tag+"-secret-admin-key")
	require.NoError(t, e)
	var snapshot bytes.Buffer
	require.NoError(t, dumpClone(ctx, tx, t.TempDir(), &snapshot))
	contents := snapshot.String()
	for _, role := range []string{"admin", "support"} {
		for _, f := range []string{"-hash-", "-key-", "-identity-"} {
			require.NotContains(t, contents, role+f+tag)
		}
	}
	for _, f := range []string{"-hash-", "-key-", "-identity-"} {
		require.Contains(t, contents, "user"+f+tag)
	}
	require.NotContains(t, contents, tag+"-secret-admin-key")
	for _, table := range []string{"security_secrets", "auth_credential_keys", "mcp_oauth_grants", "pending_auth_sessions"} {
		require.NotContains(t, contents, `"t":"`+table+`"`)
	}
	require.ErrorIs(t, validateCloneRecord(&table{name: "users"}, row{"role": json.RawMessage(`"admin"`)}), ErrDenied)
	require.ErrorIs(t, validateCloneRecord(&table{name: "settings"}, row{"key": json.RawMessage(`"admin_api_key"`)}), ErrDenied)
}

// During a rolling update, the previous binary writes clone jobs without
// the segregation marker. The database must reject even that legacy writer.
func TestOldCloneWriterRejectedByDatabase(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires a migrated test site")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer func() { _ = db.Close() }()
	tx, e := db.BeginTx(t.Context(), nil)
	require.NoError(t, e)
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(t.Context(), "INSERT INTO site_migration_jobs(id,owner,mode,profile,local_public,wrapped_private) VALUES($1,'old','export','clone','','')", uuid.NewString())
	var pg *pq.Error
	require.True(t, errors.As(e, &pg))
	require.Equal(t, "site_migration_jobs_clone_policy", pg.Constraint)
}

// Keep coverage for capability changes separate from operator-grant validation.
func TestUserKeyGrantIsExplicit(t *testing.T) {
	basic := Auth{UsersOnly: true}
	extended := Auth{UsersOnly: true, UserKeys: true}
	require.False(t, basic.Allows(userKeysProfile))
	require.True(t, extended.Allows(userKeysProfile))
	require.False(t, extended.Allows("clone"))
	require.False(t, extended.Allows("all"))
	require.True(t, Auth{}.Allows(userKeysProfile))
	require.NotEqual(t, []byte("clone"), checkpointScope("clone"))
}
