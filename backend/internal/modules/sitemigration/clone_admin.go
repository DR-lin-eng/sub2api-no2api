package sitemigration

import (
	"context"
	"database/sql"
	"errors"
)

// InitializeCloneAdmin creates a new password login on a completed, ordinary-
// user-only clone. Only the local CLI calls it, before the new site goes live.
func InitializeCloneAdmin(ctx context.Context, db *sql.DB, email, passwordHash string, concurrency int) error {
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	var job string
	e = tx.QueryRowContext(ctx, "SELECT job FROM public.private_clone_receipt WHERE finished AND ordinary_users_only AND NOT admin_initialized FOR UPDATE").Scan(&job)
	if e != nil {
		return errors.New("select a completed ordinary-user clone that has not initialized its administrator")
	}
	if _, e = tx.ExecContext(ctx, "LOCK TABLE public.users,public.auth_identities IN SHARE ROW EXCLUSIVE MODE"); e != nil {
		return e
	}
	var taken bool
	e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM public.users WHERE role<>'user' OR "+emailExpr("email")+"="+emailExpr("$1::text")+") OR EXISTS(SELECT FROM public.auth_identities WHERE provider_type='email' AND "+emailExpr("provider_subject")+"="+emailExpr("$1::text")+")", email).Scan(&taken)
	if e != nil {
		return e
	}
	if taken {
		return errors.New("an administrator exists or this email belongs to an imported account")
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO public.users(email,password_hash,role,balance,concurrency,status,created_at,updated_at) VALUES($1,$2,'admin',0,$3,'active',now(),now())", email, passwordHash, concurrency); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE public.private_clone_receipt SET admin_initialized=true WHERE job=$1", job); e != nil {
		return e
	}
	return tx.Commit()
}
