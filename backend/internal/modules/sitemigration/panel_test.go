package sitemigration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPanelPeerTrust(t *testing.T) {
	p := NewPanel(Store{}, []string{"HTTPS://site.example:9443"})
	defer p.Stop()
	require.True(t, p.allowedIP("https://site.example:9443", netip.MustParseAddr("10.1.0.5")))
	require.False(t, p.allowedIP("https://site.example", netip.MustParseAddr("10.1.0.5")))
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "::1", "fe80::1", "100.64.0.1", "::ffff:127.0.0.1", "2001:db8::1"} {
		require.False(t, p.allowedIP("https://site.example:9443", netip.MustParseAddr(ip)))
	}
	require.True(t, p.allowedIP("https://peer.example", netip.MustParseAddr("8.8.8.8")))
	for _, s := range []string{"http://site.example", "https://127.0.0.1", "https://user@site.example", "https://site.example/?key=x", "https://site.example/api", "localhost", "https://[::1]"} {
		_, e := p.peerOrigin(s, false)
		require.ErrorIs(t, e, ErrPeerAddress)
	}
}

func TestPanelHTTPRequiresExplicitOptIn(t *testing.T) {
	p := NewPanel(Store{}, []string{"http://lan.example:18080"})
	defer p.Stop()
	for _, tt := range []struct {
		input string
		allow bool
		want  string
	}{
		{"http://203.1.1.25:18080", false, ""}, {"http://203.1.1.25:18080", true, "http://203.1.1.25:18080"},
		{"https://203.1.1.25:18080", true, "https://203.1.1.25:18080"}, {"example.com:18080", true, "https://example.com:18080"},
		{"http://example.com:80", true, "http://example.com"}, {"http://example.com:443", true, "http://example.com:443"},
		{"http://127.0.0.1", true, ""}, {"http://169.254.169.254", true, ""}, {"http://example.com/path", true, ""}, {"http://name@peer.example", true, ""},
	} {
		o, err := p.peerOrigin(tt.input, tt.allow)
		if tt.want == "" {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tt.want, o)
	}
	require.True(t, p.allowedIP("http://lan.example:18080", netip.MustParseAddr("10.0.0.7")))
	require.False(t, p.allowedIP("https://lan.example:18080", netip.MustParseAddr("10.0.0.7")))
}

// Uses an isolated application DB. An incomplete report, a stale worker and
// another admin must never be able to publish or read these CSV rows.
func TestPanelReportOwnerAndAttempt(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires a migrated test site")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	p := NewPanel(Store{DB: db}, nil)
	defer p.Stop()
	id, attempt := uuid.NewString(), uuid.NewString()
	_, e = db.ExecContext(ctx, `INSERT INTO site_migration_panel_jobs(id,admin_id,direction,profile,peer,phase,attempt) VALUES($1,101,'push','login','https://peer.example','reporting',$2)`, id, attempt)
	require.NoError(t, e)
	defer func() { _, _ = db.ExecContext(ctx, "DELETE FROM site_migration_panel_jobs WHERE id=$1", id) }()
	w := &panelTransfer{p, 101, PanelJob{ID: id}, attempt}
	var got bytes.Buffer
	output := func(int64) io.Writer { return &got }
	require.ErrorIs(t, p.Report(ctx, 101, id, output), ErrConflict)
	stale := *w
	stale.attempt = uuid.NewString()
	render := func(w io.Writer) error {
		csvOut := csv.NewWriter(w)
		if err := csvOut.Write([]string{"users", "1", "private@example.test", "duplicate"}); err != nil {
			return err
		}
		csvOut.Flush()
		return csvOut.Error()
	}
	require.ErrorIs(t, stale.complete(ctx, State{}, render), ErrConflict)
	require.NoError(t, w.complete(ctx, State{Inserted: 1, Errors: 1}, render))
	require.ErrorIs(t, p.Report(ctx, 102, id, output), ErrMissing)
	require.Zero(t, got.Len())
	require.NoError(t, p.Report(ctx, 101, id, output))
	require.Contains(t, got.String(), "private@example.test")
	var encrypted []byte
	require.NoError(t, db.QueryRowContext(ctx, "SELECT ciphertext FROM site_migration_panel_reports WHERE job=$1 AND part=0", id).Scan(&encrypted))
	require.False(t, strings.Contains(string(encrypted), "private@example.test"))
	_, e = db.ExecContext(ctx, "UPDATE site_migration_panel_jobs SET expires_at=now()-interval '1 second' WHERE id=$1", id)
	require.NoError(t, e)
	require.ErrorIs(t, p.Report(ctx, 101, id, output), ErrMissing)
}
