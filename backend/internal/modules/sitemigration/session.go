// Package sitemigration transfers an immutable logical snapshot between two sites.
package sitemigration

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
)

const (
	BlockSize   = 1 << 20
	maxBlocks   = 32768 // 32 GiB of logical input per job
	maxLiveJobs = 4
)

var (
	ErrDenied   = errors.New("administrator key cannot access this migration profile")
	ErrInvalid  = errors.New("invalid migration request")
	ErrConflict = errors.New("migration state conflict")
	ErrMissing  = errors.New("migration job unavailable or expired")
	ErrLimit    = errors.New("migration transfer limit exceeded")
)

type Store struct {
	DB            *sql.DB
	DataDir       string
	BloomEnabled  bool
	CloneDatabase func(context.Context, string, string) (*sql.DB, error)
}
type Auth struct {
	Owner, Key string
	UsersOnly  bool
}

func (a Auth) Allows(profile string) bool {
	return validProfile(profile) && (!a.UsersOnly || profile == "login" || profile == "login-quota")
}

type State struct {
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	Profile  string `json:"profile"`
	Public   []byte `json:"public"`
	Blocks   int    `json:"blocks"`
	Sealed   bool   `json:"sealed"`
	Applied  bool   `json:"applied"`
	Inserted int64  `json:"inserted"`
	Errors   int    `json:"errors"`
}
type session struct {
	State
	wrapped, peer []byte
	aead          cipher.AEAD
}

func validProfile(p string) bool {
	return p == "all" || p == "clone" || p == "login" || p == "login-quota"
}
func validID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u.Version() == 4 && u.String() == id
}
func makeCipher(secret, salt []byte, info string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, secret, salt, info, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
func wrapCipher(a Auth, id, mode string) (cipher.AEAD, error) {
	return makeCipher([]byte(a.Key), []byte(id), "sub2api migration private v1/"+a.Owner+"/"+mode)
}

// Open binds a new UUID to its authenticated caller and immutable selection.
// The same handshake can be retried after a lost reply.
func (s Store) Open(ctx context.Context, a Auth, id, mode, profile string, peer []byte) (State, error) {
	if !validID(id) || !validProfile(profile) || (mode != "export" && mode != "import") || a.Owner == "" || a.Key == "" || (len(peer) != 0 && len(peer) != 32) {
		return State{}, ErrInvalid
	}
	if !a.Allows(profile) {
		return State{}, ErrDenied
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(162884213,1)"); err != nil {
		return State{}, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM site_migration_jobs WHERE expires_at <= now()"); err != nil {
		return State{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM site_migration_jobs WHERE id<>$1", id).Scan(&count); err != nil {
		return State{}, err
	}
	// Completed checkpoints are retained for receipt recovery, with a separate bound.
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM site_migration_jobs WHERE id<>$1 AND NOT applied AND NOT (mode='export' AND sealed)", id).Scan(&active); err != nil {
		return State{}, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM site_migration_jobs WHERE id=$1)", id).Scan(&exists); err != nil {
		return State{}, err
	}
	if !exists && (active >= maxLiveJobs || count >= maxLiveJobs*2) {
		return State{}, ErrLimit
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return State{}, err
	}
	wrap, err := wrapCipher(a, id, mode)
	if err != nil {
		return State{}, err
	}
	wrapped := wrap.Seal(nil, nil, key.Bytes(), []byte(profile))
	_, err = tx.ExecContext(ctx, `INSERT INTO site_migration_jobs(id,owner,mode,profile,local_public,wrapped_private,peer_public)
        VALUES($1,$2,$3,$4,$5,$6,NULLIF($7::bytea,''::bytea)) ON CONFLICT(id) DO NOTHING`, id, a.Owner, mode, profile, key.PublicKey().Bytes(), wrapped, peer)
	if err != nil {
		return State{}, err
	}
	j, err := loadJob(ctx, tx, a, id, mode)
	if err != nil {
		return State{}, err
	}
	if j.Profile != profile || (len(peer) != 0 && !bytes.Equal(j.peer, peer)) {
		return State{}, ErrConflict
	}
	return j.State, tx.Commit()
}
func loadJob(ctx context.Context, tx *sql.Tx, a Auth, id, mode string) (*session, error) {
	j := new(session)
	err := tx.QueryRowContext(ctx, `SELECT id,mode,profile,local_public,wrapped_private,peer_public,blocks,sealed,applied,inserted,errors
        FROM site_migration_jobs WHERE id=$1 AND owner=$2 AND mode=$3 AND expires_at>now() FOR UPDATE`, id, a.Owner, mode).
		Scan(&j.ID, &j.Mode, &j.Profile, &j.Public, &j.wrapped, &j.peer, &j.Blocks, &j.Sealed, &j.Applied, &j.Inserted, &j.Errors)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, err
	}
	if !a.Allows(j.Profile) {
		return nil, ErrDenied
	}
	wrap, err := wrapCipher(a, id, mode)
	if err != nil {
		return nil, err
	}
	priv, err := wrap.Open(nil, nil, j.wrapped, []byte(j.Profile))
	if err != nil {
		return nil, ErrMissing
	}
	if len(j.peer) == 0 {
		return j, nil
	}
	k, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	remote, err := ecdh.X25519().NewPublicKey(j.peer)
	if err != nil {
		return nil, ErrInvalid
	}
	shared, err := k.ECDH(remote)
	if err != nil {
		return nil, ErrInvalid
	}
	source, dest := j.Public, j.peer
	if mode == "import" {
		source, dest = dest, source
	}
	info := fmt.Sprintf("sub2api migration data v1/%s/%s/%x/%x", id, j.Profile, source, dest)
	j.aead, err = makeCipher(shared, []byte(a.Key), info)
	return j, err
}
func (s Store) withJob(ctx context.Context, a Auth, id, mode string, fn func(*sql.Tx, *session) error) (State, error) {
	return s.jobTx(ctx, a, id, mode, nil, fn)
}
func (s Store) jobTx(ctx context.Context, a Auth, id, mode string, opts *sql.TxOptions, fn func(*sql.Tx, *session) error) (State, error) {
	if !validID(id) {
		return State{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, opts)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = tx.Rollback() }()
	j, err := loadJob(ctx, tx, a, id, mode)
	if err != nil {
		return State{}, err
	}
	if err = fn(tx, j); err != nil {
		return State{}, err
	}
	return j.State, tx.Commit()
}
func (s Store) Bind(ctx context.Context, a Auth, id, mode string, peer []byte) (State, error) {
	return s.withJob(ctx, a, id, mode, func(tx *sql.Tx, j *session) error {
		if len(peer) != 32 || (len(j.peer) != 0 && !bytes.Equal(j.peer, peer)) || bytes.Equal(j.Public, peer) {
			return ErrConflict
		}
		_, err := tx.ExecContext(ctx, "UPDATE site_migration_jobs SET peer_public=$2 WHERE id=$1", id, peer)
		return err
	})
}
func (s Store) Status(ctx context.Context, a Auth, id, mode string) (State, error) {
	return s.withJob(ctx, a, id, mode, func(*sql.Tx, *session) error { return nil })
}
func (j *session) aad(kind, n int, last bool) []byte {
	return []byte(fmt.Sprintf("block-v1/%s/%d/%d/%t", j.ID, kind, n, last))
}
func (j *session) seal(kind, n int, last bool, p []byte) []byte {
	flag := byte(0)
	if last {
		flag = 1
	}
	return j.aead.Seal([]byte{flag}, nil, p, j.aad(kind, n, last))
}
func (j *session) plain(kind, n int, b []byte) ([]byte, bool, error) {
	if j.aead == nil || len(b) < 1+j.aead.Overhead() || len(b) > 1+BlockSize+j.aead.Overhead() || b[0] > 1 {
		return nil, false, ErrInvalid
	}
	last := b[0] == 1
	p, err := j.aead.Open(nil, nil, b[1:], j.aad(kind, n, last))
	if err != nil {
		return nil, false, ErrInvalid
	}
	return p, last, nil
}
func saveBlock(ctx context.Context, tx *sql.Tx, id string, kind, n int, data []byte) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO site_migration_blocks(job,kind,n,data) VALUES($1,$2,$3,$4)", id, kind, n, data)
	return err
}
func getBlock(ctx context.Context, tx *sql.Tx, id string, kind, n int) ([]byte, error) {
	var b []byte
	err := tx.QueryRowContext(ctx, "SELECT data FROM site_migration_blocks WHERE job=$1 AND kind=$2 AND n=$3", id, kind, n).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMissing
	}
	return b, err
}

type blockWriter struct {
	emit func([]byte, bool) error
	buf  []byte
}

func (w *blockWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		n := min(BlockSize-len(w.buf), len(p))
		w.buf = append(w.buf, p[:n]...)
		p = p[n:]
		if len(w.buf) == BlockSize {
			if err := w.emit(w.buf, false); err != nil {
				return total - len(p), err
			}
			w.buf = w.buf[:0]
		}
	}
	return total, nil
}
func (s Store) Prepare(ctx context.Context, a Auth, id string) (State, error) {
	return s.jobTx(ctx, a, id, "export", &sql.TxOptions{Isolation: sql.LevelRepeatableRead}, func(tx *sql.Tx, j *session) error {
		if j.Sealed {
			return nil
		}
		if j.aead == nil {
			return ErrConflict
		}
		// lib/pq cannot accept INSERTs while rows are open on the same
		// connection. Stage only sealed blocks in an unlinked 0600 file.
		spool, err := os.CreateTemp("", "sub2api-migration-")
		if err != nil {
			return err
		}
		defer func() { _ = spool.Close() }()
		if err = os.Remove(spool.Name()); err != nil {
			return err
		}
		w := blockWriter{emit: func(b []byte, last bool) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if j.Blocks >= maxBlocks {
				return ErrLimit
			}
			if _, err := spool.Write(j.seal(0, j.Blocks, last, b)); err != nil {
				return err
			}
			j.Blocks++
			return nil
		}}
		if err = s.exportSnapshot(ctx, tx, j.Profile, &w); err != nil {
			return err
		}
		if err = w.emit(w.buf, true); err != nil {
			return err
		}
		if _, err = spool.Seek(0, io.SeekStart); err != nil {
			return err
		}
		buf := make([]byte, BlockSize+1+j.aead.Overhead())
		for n := 0; n < j.Blocks; n++ {
			size := len(buf)
			if n == j.Blocks-1 {
				size = len(w.buf) + 1 + j.aead.Overhead()
			}
			if _, err = io.ReadFull(spool, buf[:size]); err != nil {
				return err
			}
			if err = saveBlock(ctx, tx, id, 0, n, buf[:size]); err != nil {
				return err
			}
		}
		j.Sealed = true
		_, err = tx.ExecContext(ctx, "UPDATE site_migration_jobs SET blocks=$2,sealed=true WHERE id=$1", id, j.Blocks)
		return err
	})
}
func (s Store) Block(ctx context.Context, a Auth, id, mode string, kind, n int) (data []byte, err error) {
	_, err = s.withJob(ctx, a, id, mode, func(tx *sql.Tx, j *session) error {
		dataBlock := kind == 0 && mode == "export" && j.Sealed && n < j.Blocks
		reportBlock := kind == 1 && mode == "import" && j.Applied && n < j.Errors
		if n < 0 || (!dataBlock && !reportBlock) {
			return ErrInvalid
		}
		var err error
		data, err = getBlock(ctx, tx, id, kind, n)
		return err
	})
	return
}
func (s Store) Receive(ctx context.Context, a Auth, id string, n int, b []byte) (State, error) {
	return s.withJob(ctx, a, id, "import", func(tx *sql.Tx, j *session) error {
		if n < 0 || n >= maxBlocks || n > j.Blocks {
			return ErrConflict
		}
		if n < j.Blocks {
			old, err := getBlock(ctx, tx, id, 0, n)
			if err != nil {
				return err
			}
			if !bytes.Equal(old, b) {
				return ErrConflict
			}
			return nil
		}
		if j.Sealed {
			return ErrConflict
		}
		p, last, err := j.plain(0, n, b)
		if err != nil {
			return err
		}
		if !last && len(p) != BlockSize {
			return ErrInvalid
		}
		if err := saveBlock(ctx, tx, id, 0, n, b); err != nil {
			return err
		}
		j.Blocks++
		j.Sealed = last
		_, err = tx.ExecContext(ctx, "UPDATE site_migration_jobs SET blocks=$2,sealed=$3 WHERE id=$1", id, j.Blocks, last)
		return err
	})
}

type blockReader struct {
	next func() ([]byte, error)
	b    []byte
}

func (r *blockReader) Read(p []byte) (int, error) {
	for len(r.b) == 0 {
		b, err := r.next()
		if err != nil {
			return 0, err
		}
		r.b = b
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
func (s Store) Apply(ctx context.Context, a Auth, id string) (State, error) {
	return s.withJob(ctx, a, id, "import", func(tx *sql.Tx, j *session) error {
		if j.Applied {
			return nil
		}
		if !j.Sealed || j.aead == nil {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(162884213,2)"); err != nil {
			return err
		}
		n := 0
		r := &blockReader{next: func() ([]byte, error) {
			if n == j.Blocks {
				return nil, io.EOF
			}
			b, err := getBlock(ctx, tx, id, 0, n)
			if err != nil {
				return nil, err
			}
			p, last, err := j.plain(0, n, b)
			if err != nil || last != (n+1 == j.Blocks) {
				return nil, ErrInvalid
			}
			n++
			return p, nil
		}}
		var err error
		if j.Profile == "clone" {
			j.Inserted, err = s.restoreClone(ctx, tx, id, r)
		} else if j.Profile == "all" && s.BloomEnabled {
			return errors.New("disable API Key Bloom prefilter on all destination nodes before merging keys")
		} else {
			err = importSnapshot(ctx, tx, j, r)
		}
		if err != nil {
			return err
		}
		j.Applied = true
		_, err = tx.ExecContext(ctx, "UPDATE site_migration_jobs SET applied=true,inserted=$2,errors=$3 WHERE id=$1", id, j.Inserted, j.Errors)
		return err
	})
}

// Keep the report protocol bounded independently of the number of rejected
// rows. Every envelope authenticates its own index, including across pages.
const reportPageRows = 128
const maxReportRow = 8192

func (s Store) ReportPage(ctx context.Context, a Auth, id string, first int) ([]byte, error) {
	var page []byte
	_, err := s.withJob(ctx, a, id, "import", func(tx *sql.Tx, j *session) error {
		if !j.Applied || first < 0 || first >= j.Errors {
			return ErrInvalid
		}
		rows, err := tx.QueryContext(ctx, "SELECT n,data FROM site_migration_blocks WHERE job=$1 AND kind=1 AND n >= $2 ORDER BY n LIMIT $3", id, first, reportPageRows)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var n int
			var b []byte
			if err = rows.Scan(&n, &b); err != nil {
				return err
			}
			if n != first || len(b) > maxReportRow+j.aead.Overhead()+1 {
				return ErrInvalid
			}
			if len(page)+4+len(b) > BlockSize {
				break
			}
			page = binary.BigEndian.AppendUint32(page, uint32(len(b)))
			page = append(page, b...)
			first++
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(page) == 0 {
			return ErrMissing
		}
		return nil
	})
	return page, err
}

func (s Store) DecodeReportPage(ctx context.Context, a Auth, id, mode string, first, remaining int, page []byte, write func([]byte) error) (int, error) {
	count := 0
	_, err := s.withJob(ctx, a, id, mode, func(tx *sql.Tx, j *session) error {
		if len(page) == 0 || len(page) > BlockSize || first < 0 {
			return ErrInvalid
		}
		for len(page) > 0 {
			if len(page) < 4 || count >= reportPageRows || count >= remaining {
				return ErrInvalid
			}
			n := uint64(binary.BigEndian.Uint32(page))
			page = page[4:]
			if n > uint64(len(page)) || n > maxReportRow+64 {
				return ErrInvalid
			}
			b, last, err := j.plain(1, first+count, page[:n])
			if err != nil || !last || len(b) > maxReportRow {
				return ErrInvalid
			}
			if err = write(b); err != nil {
				return err
			}
			page = page[n:]
			count++
		}
		return nil
	})
	return count, err
}
func (s Store) Remove(ctx context.Context, a Auth, id, mode string) (State, error) {
	return s.withJob(ctx, a, id, mode, func(tx *sql.Tx, j *session) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM site_migration_jobs WHERE id=$1", id)
		return err
	})
}

func (s Store) Existing(ctx context.Context) (bool, error) {
	var found bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM public.users UNION ALL SELECT FROM public.groups UNION ALL SELECT FROM public.settings)`).Scan(&found)
	return found, err
}
