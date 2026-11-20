//go:build integration

package sitemigration

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Give this test an isolated, migrated Sub2API fixture. A one-connection pool
// must complete a prepare and its persisted receipt without depending on a
// second DB slot. No user or other business rows are modified.
func TestMigrationTransportSnapshotSingleConnection(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires an isolated migrated site")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	s := Store{DB: db}
	a := Auth{Owner: "single-pool-test", Key: "synthetic only"}
	peer, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	_, err = s.Open(ctx, a, id, "export", "login", peer.PublicKey().Bytes())
	require.NoError(t, err)
	defer func() { _, err := s.Remove(context.Background(), a, id, "export"); require.NoError(t, err) }()
	job, err := s.Prepare(ctx, a, id)
	require.NoError(t, err)
	require.True(t, job.Sealed)
	require.Positive(t, job.Blocks)
	for n := 0; n < job.Blocks; n++ {
		payload, e := s.Block(ctx, a, id, "export", 0, n)
		require.NoError(t, e)
		require.Equal(t, n == job.Blocks-1, payload[0] == 1)
	}
	again, err := s.Prepare(ctx, a, id)
	require.NoError(t, err)
	require.Equal(t, job, again)
}
