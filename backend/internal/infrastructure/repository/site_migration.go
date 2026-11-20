package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func ProvideSiteMigration(db *sql.DB, cfg *config.Config) sitemigration.Store {
	return sitemigration.Store{DB: db, DataDir: cfg.Pricing.DataDir, BloomEnabled: cfg.APIKeyAuth.Bloom.Enabled, CloneDatabase: func(ctx context.Context, name, id string) (*sql.DB, error) {
		return provisionMigrationDatabase(ctx, cfg.Database, name, id)
	}}
}

// A marker generated and persisted by the receiver authorizes retries. The
// untrusted job UUID alone never authorizes access to an existing database.
func provisionMigrationDatabase(ctx context.Context, configDB config.DatabaseConfig, name, id string) (*sql.DB, error) {
	job, err := uuid.Parse(id)
	if err != nil || job.Version() != 4 || job.String() != id || name != "sub2api_clone_"+strings.ReplaceAll(id, "-", "") || name == configDB.DBName {
		return nil, errors.New("invalid clone database")
	}
	control, err := sql.Open("postgres", configDB.DSN())
	if err != nil {
		return nil, err
	}
	defer control.Close()
	control.SetMaxOpenConns(1)
	// The outer transfer already serializes apply operations. Commit this
	// reservation independently so a lost connection after CREATE is recoverable.
	var marker string
	err = control.QueryRowContext(ctx, "SELECT marker FROM public.site_migration_clones WHERE job=$1", id).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if err = control.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM pg_database WHERE datname=$1)", name).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("clone database name already in use")
		}
		if err = control.QueryRowContext(ctx, "INSERT INTO public.site_migration_clones(job) VALUES($1) RETURNING marker", id).Scan(&marker); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if _, err = control.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)+" TEMPLATE template0"); err != nil {
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "42P04" {
			return nil, fmt.Errorf("cannot create isolated database (CREATEDB required): %w", err)
		}
	}
	configDB.DBName = name
	target, err := sql.Open("postgres", configDB.DSN())
	if err != nil {
		return nil, err
	}
	target.SetMaxOpenConns(4)
	if err = bindMigrationDatabase(ctx, target, id, name, marker); err == nil {
		err = ApplyMigrations(ctx, target)
	}
	if err != nil {
		target.Close()
		return nil, err
	}
	return target, nil
}
func bindMigrationDatabase(ctx context.Context, db *sql.DB, id, name, marker string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner, receipt bool
	if err = tx.QueryRowContext(ctx, `SELECT d.datdba=(SELECT oid FROM pg_roles WHERE rolname=current_user),to_regclass('public.private_clone_receipt') IS NOT NULL FROM pg_database d WHERE d.datname=current_database()`).Scan(&owner, &receipt); err != nil {
		return err
	}
	if !owner {
		return errors.New("clone database owner mismatch")
	}
	if !receipt {
		var occupied bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM pg_namespace n WHERE n.nspname NOT IN ('pg_catalog','information_schema','public') AND n.nspname !~ '^pg_')
          OR EXISTS(SELECT FROM pg_class WHERE relnamespace='public'::regnamespace)
          OR EXISTS(SELECT FROM pg_proc WHERE pronamespace='public'::regnamespace)
          OR EXISTS(SELECT FROM pg_type WHERE typnamespace='public'::regnamespace)`).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return errors.New("unrecognized non-empty clone database")
		}
		if _, err = tx.ExecContext(ctx, `CREATE TABLE public.private_clone_receipt(job uuid PRIMARY KEY, marker uuid NOT NULL, finished boolean NOT NULL DEFAULT false,inserted bigint NOT NULL DEFAULT 0)`); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO public.private_clone_receipt(job,marker) VALUES($1,$2)", id, marker); err != nil {
			return err
		}
	} else {
		var ok bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM public.private_clone_receipt WHERE job=$1 AND marker=$2)", id, marker).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return errors.New("clone database ownership marker mismatch")
		}
	}
	if _, err = tx.ExecContext(ctx, "REVOKE ALL PRIVILEGES ON DATABASE "+pq.QuoteIdentifier(name)+" FROM PUBLIC"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "GRANT CONNECT, TEMPORARY ON DATABASE "+pq.QuoteIdentifier(name)+" TO CURRENT_USER"); err != nil {
		return err
	}
	return tx.Commit()
}
