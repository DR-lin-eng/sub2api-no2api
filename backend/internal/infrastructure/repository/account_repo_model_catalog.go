package repository

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
)

// UpdateUpstreamModelCatalog atomically replaces a complete successful snapshot
// and OAuth ID list. Identity and refresh generation prevent late refreshes from
// overwriting newer results or attaching one account's metadata to another.
func (r *accountRepository) UpdateUpstreamModelCatalog(ctx context.Context, account *service.Account, snapshot *modelcatalog.Snapshot) error {
	if account == nil || snapshot == nil || snapshot.Version != modelcatalog.Version {
		return service.ErrAccountNilInput
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > modelcatalog.MaxSnapshotBytes || len(snapshot.Models) == 0 || len(snapshot.Models) > modelcatalog.MaxModels {
		return errors.New("invalid model catalog snapshot")
	}
	updates := map[string]any{modelcatalog.ExtraKey: snapshot}
	if account.IsOAuth() {
		ids := make([]string, 0, len(snapshot.Models))
		for id := range snapshot.Models {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		updates[service.OAuthSupportedModelsExtraKey] = ids
		updates[service.OAuthSupportedModelsSyncedAtExtraKey] = snapshot.SyncedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	payload, err := json.Marshal(updates)
	if err != nil {
		return err
	}
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return err
	}
	baseCtx := ctx
	var tx *dbent.Tx
	if dbent.TxFromContext(ctx) == nil {
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
		}
	}
	client := clientFromContext(ctx, r.client)
	proxyMatches, err := lockAndMatchProbeProxyIdentity(ctx, client, account)
	if err != nil {
		return err
	}
	if !proxyMatches {
		return service.ErrUpstreamModelCatalogChanged
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	result, err := client.ExecContext(ctx, `UPDATE accounts
 SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb, updated_at = NOW()
 WHERE id = $2 AND platform = $3 AND type = $4
 AND credentials = $5::jsonb AND proxy_id IS NOT DISTINCT FROM $6
 AND deleted_at IS NULL
 AND (extra -> 'upstream_model_metadata' IS NULL OR
  CASE WHEN extra -> 'upstream_model_metadata' ->> 'version' = '1'
  THEN COALESCE((extra -> 'upstream_model_metadata' ->> 'generation')::numeric, 0) < $7
  ELSE false END)`, string(payload), account.ID, account.Platform, account.Type, string(credentials), proxyID, snapshot.Generation)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUpstreamModelCatalogChanged
	}
	// OAuth capability changes affect scheduling; metadata-only refreshes use
	// the same outbox so every instance sees an account-scoped fresh snapshot.
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, nil); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
		r.syncSchedulerAccountSnapshot(baseCtx, account.ID)
	}
	return nil
}
