package sitemigration

import (
	"context"
	"crypto/cipher"
	"database/sql"
	"fmt"
	"io"
)

func reportCipher(ctx context.Context, tx *sql.Tx, id string, admin int64) (cipher.AEAD, error) {
	secret, e := storageKey(ctx, tx)
	if e != nil {
		return nil, e
	}
	return makeCipher(secret, []byte(id), fmt.Sprintf("sub2api panel CSV v1/%d", admin))
}
func reportAAD(part int) []byte { return []byte(fmt.Sprintf("csv/%d", part)) }

// Partial generations are private to their attempt. Publishing the final size
// and phase makes only the completed generation downloadable on all replicas.
func (w *panelTransfer) complete(ctx context.Context, dst State, render func(io.Writer) error) error {
	db := w.panel.store.DB
	// CSV generation reads the wire checkpoint. Release DB connections before
	// callbacks so a single-connection pool can also produce large reports.
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	aead, e := reportCipher(ctx, tx, w.job.ID, w.admin)
	_ = tx.Rollback()
	if e != nil {
		return e
	}
	n := 0
	var size int64
	out := blockWriter{emit: func(b []byte, _ bool) error {
		if len(b) == 0 {
			return nil
		}
		res, err := db.ExecContext(ctx, "INSERT INTO site_migration_panel_reports(job,attempt,part,ciphertext) SELECT id,attempt,$3,$4 FROM site_migration_panel_jobs WHERE id=$1 AND attempt=$2", w.job.ID, w.attempt, n, aead.Seal(nil, nil, b, reportAAD(n)))
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
		size += int64(len(b))
		n++
		return nil
	}}
	if e = render(&out); e != nil {
		return e
	}
	if e = out.emit(out.buf, true); e != nil {
		return e
	}
	return w.update(ctx, "UPDATE site_migration_panel_jobs SET phase='completed',lease_until=NULL,report_bytes=$3,inserted=$4,issues=$5,expires_at=now()+interval '24 hours' WHERE id=$1 AND attempt=$2", size, dst.Inserted, dst.Errors)
}
func (p *Panel) Report(ctx context.Context, admin int64, id string, begin func(int64) io.Writer) error {
	if !validID(id) {
		return ErrInvalid
	}
	tx, e := p.store.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	j, e := scanPanelJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM site_migration_panel_jobs WHERE id=$1 AND admin_id=$2 AND expires_at>now()", id, admin))
	if e != nil {
		return e
	}
	if j.Phase != "completed" || j.ReportBytes <= 0 {
		return ErrConflict
	}
	aead, e := reportCipher(ctx, tx, id, admin)
	if e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, "SELECT part,ciphertext FROM site_migration_panel_reports WHERE job=$1 AND attempt=(SELECT attempt FROM site_migration_panel_jobs WHERE id=$1) ORDER BY part", id)
	if e != nil {
		return e
	}
	defer func() { _ = rows.Close() }()
	var w io.Writer
	var size int64
	index := 0
	for rows.Next() {
		var n int
		var enc []byte
		if e = rows.Scan(&n, &enc); e != nil {
			return e
		}
		if n != index {
			return ErrInvalid
		}
		index++
		b, err := aead.Open(nil, nil, enc, reportAAD(n))
		if err != nil {
			return ErrInvalid
		}
		if w == nil {
			w = begin(j.ReportBytes)
		}
		if _, err = w.Write(b); err != nil {
			return err
		}
		size += int64(len(b))
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if size != j.ReportBytes {
		return ErrInvalid
	}
	return nil
}
