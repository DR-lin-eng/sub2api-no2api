package sitemigration

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Exercise both ends of the handshake with their own wrapped private keys.
// All fixtures are synthetic and the DB returns only in-memory test rows.
func TestMigrationTransportHandshakeAndReportPages(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	id := uuid.NewString()
	source, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	dest, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	push := Auth{Owner: "cli/https://example.test", Key: "fixture secret only"}
	recv := Auth{Owner: "remote/fixture", Key: push.Key}
	fields := []string{"id", "mode", "profile", "local_public", "wrapped_private", "peer_public", "blocks", "sealed", "applied", "inserted", "errors"}
	job := func(a Auth, mode string, local *ecdh.PrivateKey, peer []byte) {
		w, e := wrapCipher(a, id, mode)
		require.NoError(t, e)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,mode,profile").WithArgs(id, a.Owner, mode).WillReturnRows(
			sqlmock.NewRows(fields).AddRow(id, mode, "all", local.PublicKey().Bytes(), w.Seal(nil, nil, local.Bytes(), []byte("all")), peer, 1, true, true, 1, 2))
	}
	read := func(a Auth, mode string, k *ecdh.PrivateKey, peer []byte) *session {
		job(a, mode, k, peer)
		tx, e := db.Begin()
		require.NoError(t, e)
		j, e := loadJob(t.Context(), tx, a, id, mode)
		require.NoError(t, e)
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		return j
	}
	require.Nil(t, read(push, "export", source, []byte{}).aead)
	sender := read(push, "export", source, dest.PublicKey().Bytes())
	receiver := read(recv, "import", dest, source.PublicKey().Bytes())
	sample := []byte("fictional transfer record")
	block := sender.seal(0, 0, true, sample)
	p, last, err := receiver.plain(0, 0, block)
	require.NoError(t, err)
	require.True(t, last)
	require.Equal(t, sample, p)
	_, _, err = receiver.plain(0, 1, block)
	require.ErrorIs(t, err, ErrInvalid)
	_, _, err = receiver.plain(1, 0, block)
	require.ErrorIs(t, err, ErrInvalid)
	changed := bytes.Clone(block)
	changed[0] = 0
	_, _, err = receiver.plain(0, 0, changed)
	require.ErrorIs(t, err, ErrInvalid)
	// Key metadata is freshly checked and a demoted key cannot reuse a full checkpoint.
	limited := recv
	limited.UsersOnly = true
	job(recv, "import", dest, source.PublicKey().Bytes())
	mock.ExpectRollback()
	_, err = (Store{DB: db}).Status(t.Context(), limited, id, "import")
	require.ErrorIs(t, err, ErrDenied)
	var page []byte
	for i, data := range []string{"users,1,owner@example.test,duplicate\n", "users,2,other@example.test,duplicate\n"} {
		b := receiver.seal(1, i, true, []byte(data))
		page = binary.BigEndian.AppendUint32(page, uint32(len(b)))
		page = append(page, b...)
	}
	var lines int
	job(push, "export", source, dest.PublicKey().Bytes())
	mock.ExpectCommit()
	n, err := (Store{DB: db}).DecodeReportPage(t.Context(), push, id, "export", 0, 2, page, func([]byte) error { lines++; return nil })
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 2, lines)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrationTransportCSVAndDestination(t *testing.T) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	require.NoError(t, writeReportRecord(w, []byte("users,1,=9+9,原因\n")))
	w.Flush()
	require.Contains(t, b.String(), "'=9+9")
	require.ErrorIs(t, writeReportRecord(w, []byte("users,2,a\n")), ErrInvalid)
	require.ErrorIs(t, writeReportRecord(w, []byte("a,b,c,d\nw,x,y,z\n")), ErrInvalid)
	dir := t.TempDir()
	p := filepath.Join(dir, "report.csv")
	require.NoError(t, os.WriteFile(p, []byte("original"), 0600))
	require.Error(t, writeReport(p, func(wrt io.Writer) error { _, err := wrt.Write(b.Bytes()); return err }))
	old, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original", string(old))
	for _, bad := range []string{"http://example.test", "https://user@example.test", "https://example.test/path", "https://example.test?test=1"} {
		_, err := peerOrigin(bad)
		require.Error(t, err)
	}
	u, err := peerOrigin("[2001:db8::1]:8443")
	require.NoError(t, err)
	require.Equal(t, "https://[2001:db8::1]:8443", u)
}
