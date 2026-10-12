package repository

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestSecurityH2S3RejectsLoopbackWithoutNetworkRequest(t *testing.T) {
	var contacted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	store, err := NewS3BackupStoreFactory()(t.Context(), &service.BackupS3Config{
		Endpoint: server.URL, Region: "us-east-1", Bucket: "test", AccessKeyID: "fixture", SecretAccessKey: "fixture", ForcePathStyle: true,
	})
	if err == nil {
		err = store.HeadBucket(t.Context())
	}
	t.Logf("H2 loopback contacted=%d rejected=%t", contacted.Load(), err != nil)
	require.Error(t, err)
	require.Zero(t, contacted.Load())
}

// Set BACKUP_RESTORE_TEST_DATABASE only for a disposable PostgreSQL database.
// The psql and pg_dump clients must connect using the local socket as postgres.
func TestSecurityH3RestoreWithPsql(t *testing.T) {
	database := os.Getenv("BACKUP_RESTORE_TEST_DATABASE")
	if database == "" {
		t.Skip("requires disposable PostgreSQL and psql/pg_dump clients")
	}
	psql := func(sql string) string {
		t.Helper()
		out, err := exec.Command("psql", "-X", "-U", "postgres", "-d", database, "-At", "-v", "ON_ERROR_STOP=1", "-c", sql).CombinedOutput()
		require.NoError(t, err, "%s", out)
		return strings.TrimSpace(string(out))
	}
	dumper := &PgDumper{cfg: &config.DatabaseConfig{Host: "/var/run/postgresql", Port: 5432, User: "postgres", DBName: database}}
	for _, payload := range []struct{ name, script string }{
		{"shell", "\\! touch /tmp/h123_restore_marker\nSELECT 1;\n"},
		{"copy_program", "\\copy (SELECT 1) TO PROGRAM 'touch /tmp/h123_restore_marker'\n"},
		{"output_pipe", "\\o |touch /tmp/h123_restore_marker\nSELECT 1;\n"},
		{"unlock", "\\unrestrict attackerKnownKey\n\\! touch /tmp/h123_restore_marker\n"},
		{"unlock_backtick", "\\unrestrict `touch /tmp/h123_restore_marker`\n"},
		{"wrapped_shell", "-- PostgreSQL database dump\n\n\\restrict attackerKnownKey\n\\! touch /tmp/h123_restore_marker\n\\unrestrict attackerKnownKey\n"},
	} {
		t.Run(payload.name, func(t *testing.T) {
			psql("\\! rm -f /tmp/h123_restore_marker")
			err := dumper.Restore(t.Context(), strings.NewReader(payload.script))
			marker := psql("\\! if test -f /tmp/h123_restore_marker; then printf present; else printf absent; fi")
			t.Logf("H3 %s rejected=%t shell_marker=%s", payload.name, err != nil, marker)
			require.Equal(t, "absent", marker)
			require.Error(t, err)
		})
	}
	t.Run("legacy_copy", func(t *testing.T) {
		psql("DROP TABLE IF EXISTS h123_restore_data")
		err := dumper.Restore(t.Context(), strings.NewReader("CREATE TABLE h123_restore_data(value text);\nCOPY h123_restore_data FROM stdin;\nhello\\tworld\n\\\\restrict literal_data\n\\.\n"))
		require.NoError(t, err)
		require.Equal(t, "2", psql("SELECT count(*) FROM h123_restore_data"))
		t.Log("H3 legacy SQL and COPY restored rows=2")
	})
	t.Run("error_rolls_back", func(t *testing.T) {
		psql("DROP TABLE IF EXISTS h123_restore_failure")
		err := dumper.Restore(t.Context(), strings.NewReader("CREATE TABLE h123_restore_failure(id int);\nSELECT nonexistent_h123_function();\n"))
		state := psql("SELECT to_regclass('h123_restore_failure') IS NULL")
		t.Logf("H3 SQL error rejected=%t transaction_rolled_back=%s", err != nil, state)
		require.Error(t, err)
		require.Equal(t, "t", state)
	})
	t.Run("pg_dump_round_trip", func(t *testing.T) {
		psql("TRUNCATE h123_restore_data; INSERT INTO h123_restore_data VALUES (repeat('x', 131072) || chr(92) || 'restrict data')")
		backup, err := exec.Command("pg_dump", "-U", "postgres", "-d", database, "--no-owner", "--no-acl", "--clean", "--if-exists").Output()
		require.NoError(t, err)
		psql("TRUNCATE h123_restore_data")
		err = dumper.Restore(t.Context(), strings.NewReader(string(backup)))
		require.NoError(t, err)
		require.Equal(t, fmt.Sprint(131072+len("\\restrict data")), psql("SELECT length(value) FROM h123_restore_data"))
		t.Log("H3 PostgreSQL pg_dump round trip and >64 KiB COPY row preserved")
	})
}
