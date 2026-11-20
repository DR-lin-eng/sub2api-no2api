package sitemigration

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// New-site replication restores business IDs and relations in a separate database.
// Portable credentials are rewrapped with a new, receiver-generated key.
const cloneExtraTables = `site_migration_studio_rebinds usage_billing_jobs usage_cleanup_tasks
prompt_audit_jobs prompt_audit_queue_state usage_dashboard_aggregation_watermark channel_monitor_aggregation_watermark usage_group_rollup_state`

func cloneTopology(c catalog) ([]string, error) {
	names := make([]string, 0, len(c.tables))
	for name := range c.tables {
		names = append(names, name)
	}
	slices.Sort(names)
	var order []string
	visited := map[string]bool{}
	for len(order) < len(names) {
		n := len(order)
		for _, name := range names {
			if visited[name] {
				continue
			}
			if slices.ContainsFunc(c.tables[name].refs, func(r ref) bool { return r.parent != name && c.tables[r.parent] != nil && !visited[r.parent] }) {
				continue
			}
			visited[name] = true
			order = append(order, name)
		}
		if n == len(order) {
			return nil, errors.New("cyclic clone schema")
		}
	}
	return order, nil
}

type seqValue struct {
	Table  string
	Last   int64
	Called bool
}
type cloneFrame struct {
	Table    string    `json:"t,omitempty"`
	Row      row       `json:"r,omitempty"`
	Sequence *seqValue `json:"s,omitempty"`
	File     *pagePart `json:"f,omitempty"`
}

func dumpClone(ctx context.Context, tx *sql.Tx, dataDir string, w io.Writer) error {
	c, err := readCatalog(ctx, tx, "clone")
	if err != nil {
		return err
	}
	var site string
	if err = tx.QueryRowContext(ctx, "SELECT site FROM site_migration_identity").Scan(&site); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	if err = enc.Encode(header{2, site, c.hash, "clone"}); err != nil {
		return err
	}
	queries, err := cloneQueries(ctx, tx, c)
	if err != nil {
		return err
	}
	key, err := storageKey(ctx, tx)
	if err != nil {
		return err
	}
	for _, name := range c.order {
		t := c.tables[name]
		q := queries[name]
		rows, e := tx.QueryContext(ctx, q)
		if e != nil {
			return e
		}
		for rows.Next() {
			var b json.RawMessage
			if e = rows.Scan(&b); e == nil {
				if len(b) > maxRow/2 {
					e = ErrLimit
				} else {
					var record row
					if e = json.Unmarshal(b, &record); e == nil {
						e = cloneSecrets(name, record, key, true)
					}
					if e == nil {
						e = enc.Encode(cloneFrame{Table: name, Row: record})
					}
				}
			}
			if e != nil {
				_ = rows.Close()
				return e
			}
		}
		if e = rows.Err(); e != nil {
			_ = rows.Close()
			return e
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if t.serial {
			seq, e := sequenceName(ctx, tx, name)
			if e != nil {
				return e
			}
			seqState := seqValue{Table: name}
			if e = tx.QueryRowContext(ctx, "SELECT last_value,is_called FROM "+seq).Scan(&seqState.Last, &seqState.Called); e != nil {
				return e
			}
			if e = enc.Encode(cloneFrame{Sequence: &seqState}); e != nil {
				return e
			}
		}
	}
	return exportPages(dataDir, enc)
}
func sequenceName(ctx context.Context, tx *sql.Tx, table string) (string, error) {
	var seq string
	e := tx.QueryRowContext(ctx, "SELECT c.relname FROM pg_class c WHERE c.oid=pg_get_serial_sequence($1,'id')::regclass AND c.relnamespace='public'::regnamespace", qt(table)).Scan(&seq)
	return qt(seq), e
}

func (s Store) cloneName(id string) string { return "sub2api_clone_" + strings.ReplaceAll(id, "-", "") }
func (s Store) restoreClone(ctx context.Context, live *sql.Tx, id string, input io.Reader) (int64, error) {
	if s.CloneDatabase == nil {
		return 0, errors.New("new database provisioning is unavailable")
	}
	local, err := readCatalog(ctx, live, "clone")
	if err != nil {
		return 0, err
	}
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 64<<10), maxRow)
	var h header
	if !scan.Scan() || json.Unmarshal(scan.Bytes(), &h) != nil || h.Version != 2 || h.Profile != "clone" || !validID(h.Site) || h.Schema != local.hash {
		return 0, errors.New("clone schema mismatch")
	}
	var same bool
	if err = live.QueryRowContext(ctx, "SELECT site=$1 FROM site_migration_identity", h.Site).Scan(&same); err != nil {
		return 0, err
	}
	if same {
		return 0, errors.New("cannot clone a site to itself")
	}
	// The callback creates a dedicated empty database, installs receiver-owned
	// migrations, and binds it to this job using an unguessable local marker.
	db, err := s.CloneDatabase(ctx, s.cloneName(id), id)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var done bool
	var count int64
	if err = tx.QueryRowContext(ctx, "SELECT finished,inserted FROM private_clone_receipt WHERE job=$1 AND (NOT finished OR ordinary_users_only) FOR UPDATE", id).Scan(&done, &count); err != nil {
		return 0, err
	}
	if done {
		if err := verifyPublishedPages(s.DataDir, id); err != nil {
			return 0, fmt.Errorf("published clone pages failed verification: %w", err)
		}
		return count, nil
	}
	c, err := readCatalog(ctx, tx, "clone")
	if err != nil {
		return 0, err
	}
	if c.hash != h.Schema {
		return 0, errors.New("new clone database schema mismatch")
	}
	// The destination is a private, newly provisioned DB, never the live DB.
	// Clearing migration seed rows makes the source authoritative for these tables.
	names := make([]string, 0, len(c.order))
	for _, name := range c.order {
		names = append(names, qt(name))
	}
	if _, err = tx.ExecContext(ctx, "TRUNCATE "+strings.Join(names, ",")+" RESTART IDENTITY CASCADE"); err != nil {
		return 0, err
	}
	// Disable only application triggers; PostgreSQL keeps every FK enabled.
	for _, name := range c.order {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE "+qt(name)+" DISABLE TRIGGER USER"); err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE clone_rows(n bigint GENERATED ALWAYS AS IDENTITY,t text NOT NULL,v jsonb NOT NULL) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	key, err := cloneStorageKey(ctx, tx)
	if err != nil {
		return 0, err
	}
	pages, err := newPageRestore(s.DataDir, id)
	if err != nil {
		return 0, err
	}
	defer pages.close()
	sequences := map[string]seqValue{}
	var batch []string
	size := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO clone_rows(t,v) SELECT x->>'t',x->'r' FROM jsonb_array_elements($1::jsonb) AS x", "["+strings.Join(batch, ",")+"]")
		batch = nil
		size = 0
		return e
	}
	for scan.Scan() {
		var f cloneFrame
		if json.Unmarshal(scan.Bytes(), &f) != nil {
			return 0, ErrInvalid
		}
		switch {
		case f.File != nil:
			if err = pages.write(*f.File); err != nil {
				return 0, err
			}
		case f.Sequence != nil:
			v := *f.Sequence
			t := c.tables[v.Table]
			if t == nil || !t.serial || v.Last < 1 || v.Last >= 1<<62 {
				return 0, ErrInvalid
			}
			if _, ok := sequences[t.name]; ok {
				return 0, ErrInvalid
			}
			sequences[t.name] = v
		default:
			t := c.tables[f.Table]
			if t == nil || len(f.Row) != len(t.cols) {
				return 0, ErrInvalid
			}
			for _, col := range t.cols {
				if f.Row[col] == nil {
					return 0, ErrInvalid
				}
			}
			if err = validateCloneRecord(t, f.Row); err != nil {
				return 0, err
			}
			if err = cloneSecrets(t.name, f.Row, key, false); err != nil {
				return 0, err
			}
			value, e := json.Marshal(f)
			if e != nil {
				return 0, e
			}
			batch = append(batch, string(value))
			size += len(value)
			count++
			if size >= BlockSize || len(batch) >= 256 {
				if err = flush(); err != nil {
					return 0, err
				}
			}
		}
	}
	if err = scan.Err(); err != nil {
		return 0, err
	}
	if err = flush(); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "CREATE INDEX ON clone_rows(t,n); ANALYZE clone_rows"); err != nil {
		return 0, err
	}
	order, err := cloneTopology(c)
	if err != nil {
		return 0, err
	}
	for _, name := range order {
		t := c.tables[name]
		cols := columns(t.cols)
		q := "INSERT INTO " + qt(name) + " (" + cols + ") SELECT x.* FROM pg_temp.clone_rows r CROSS JOIN LATERAL jsonb_populate_record(NULL::" + qt(name) + ",r.v) AS x WHERE r.t=$1 ORDER BY r.n"
		if _, err = tx.ExecContext(ctx, q, name); err != nil {
			return 0, fmt.Errorf("cannot restore %s", name)
		}
		if t.serial {
			v, ok := sequences[name]
			if !ok {
				return 0, ErrInvalid
			}
			seq, e := sequenceName(ctx, tx, name)
			if e != nil {
				return 0, e
			}
			// ALTER SEQUENCE RESTART participates in the transaction, unlike setval().
			next := v.Last
			if v.Called {
				next++
			}
			var maxID int64
			if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id),0) FROM "+qt(name)).Scan(&maxID); err != nil {
				return 0, err
			}
			next = max(next, maxID+1)
			if _, err = tx.ExecContext(ctx, "ALTER SEQUENCE "+seq+" RESTART WITH "+strconv.FormatInt(next, 10)); err != nil {
				return 0, err
			}
		}
		if _, err = tx.ExecContext(ctx, "ALTER TABLE "+qt(name)+" ENABLE TRIGGER USER"); err != nil {
			return 0, err
		}
	}
	if err = pages.publish(); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE private_clone_receipt SET finished=true,ordinary_users_only=true,inserted=$2 WHERE job=$1", id, count); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// A clone cannot publish pages into the live site's served directory.
func clonePageDirectory(root, id string) string {
	return filepath.Join(root, "site-migration-clones", id, "pages")
}
