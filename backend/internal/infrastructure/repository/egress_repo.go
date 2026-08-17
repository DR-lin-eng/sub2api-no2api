package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbbinding "github.com/Wei-Shaw/sub2api/ent/accountegressbinding"
	dbpool "github.com/Wei-Shaw/sub2api/ent/ipv6egresspool"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

const egressPoolAdvisoryLock int64 = 0x6970763665677265

type egressRepository struct {
	client         *dbent.Client
	sql            sqlExecutor
	schedulerCache service.SchedulerCache
}

func NewEgressStore(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) moduleegress.Store {
	return &egressRepository{client: client, sql: sqlDB, schedulerCache: schedulerCache}
}

func (r *egressRepository) CreatePool(ctx context.Context, input moduleegress.CreatePoolInput) (*moduleegress.Pool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Client().ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", egressPoolAdvisoryLock); err != nil {
		return nil, err
	}
	rows, err := tx.Client().QueryContext(ctx, `SELECT name FROM ipv6_egress_pools WHERE cidr::cidr && $1::cidr LIMIT 1`, input.CIDR)
	if err != nil {
		return nil, err
	}
	var overlappingName string
	if rows.Next() {
		if err := rows.Scan(&overlappingName); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if overlappingName != "" {
		return nil, fmt.Errorf("%w: %s", moduleegress.ErrPoolOverlap, overlappingName)
	}
	if input.IsDefault {
		if _, err := tx.IPv6EgressPool.Update().SetIsDefault(false).Save(ctx); err != nil {
			return nil, err
		}
	}
	builder := tx.IPv6EgressPool.Create().
		SetName(strings.TrimSpace(input.Name)).
		SetCidr(strings.TrimSpace(input.CIDR)).
		SetStatus(moduleegress.PoolStatusActive).
		SetIsDefault(input.IsDefault).
		SetAllocationVersion(1)
	if input.NodeID != nil && strings.TrimSpace(*input.NodeID) != "" {
		builder.SetNodeID(strings.TrimSpace(*input.NodeID))
	}
	entity, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return poolEntityToModule(entity, 0), nil
}

func (r *egressRepository) UpdatePool(ctx context.Context, id int64, input moduleegress.UpdatePoolInput) (*moduleegress.Pool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.IPv6EgressPool.Query().Where(dbpool.IDEQ(id)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, moduleegress.ErrPoolNotFound
		}
		return nil, err
	}
	builder := tx.IPv6EgressPool.UpdateOne(current)
	if input.Name != nil {
		builder.SetName(strings.TrimSpace(*input.Name))
	}
	if input.NodeID != nil {
		if nodeID := strings.TrimSpace(*input.NodeID); nodeID != "" {
			builder.SetNodeID(nodeID)
		} else {
			builder.ClearNodeID()
		}
	}
	if input.Status != nil {
		builder.SetStatus(*input.Status)
		if *input.Status == moduleegress.PoolStatusDisabled {
			builder.SetIsDefault(false)
		}
	}
	if input.IsDefault != nil {
		if *input.IsDefault {
			status := current.Status
			if input.Status != nil {
				status = *input.Status
			}
			if status != moduleegress.PoolStatusActive {
				return nil, moduleegress.ErrPoolDisabled
			}
			if _, err := tx.IPv6EgressPool.Update().Where(dbpool.IDNEQ(id)).SetIsDefault(false).Save(ctx); err != nil {
				return nil, err
			}
		}
		builder.SetIsDefault(*input.IsDefault)
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	var accountIDs []int64
	if err := tx.AccountEgressBinding.Query().Where(dbbinding.PoolIDEQ(id)).Select(dbbinding.FieldAccountID).Scan(ctx, &accountIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.notifyAccounts(ctx, accountIDs)
	count, _ := r.client.AccountEgressBinding.Query().Where(dbbinding.PoolIDEQ(id)).Count(ctx)
	return poolEntityToModule(updated, int64(count)), nil
}

func (r *egressRepository) DeletePool(ctx context.Context, id int64) error {
	count, err := r.client.AccountEgressBinding.Query().Where(dbbinding.PoolIDEQ(id)).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return moduleegress.ErrPoolInUse
	}
	deleted, err := r.client.IPv6EgressPool.Delete().Where(dbpool.IDEQ(id)).Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return moduleegress.ErrPoolNotFound
	}
	return nil
}

func (r *egressRepository) GetPool(ctx context.Context, id int64) (*moduleegress.Pool, error) {
	entity, err := r.client.IPv6EgressPool.Query().Where(dbpool.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, moduleegress.ErrPoolNotFound
		}
		return nil, err
	}
	count, err := entity.QueryBindings().Count(ctx)
	if err != nil {
		return nil, err
	}
	return poolEntityToModule(entity, int64(count)), nil
}

func (r *egressRepository) GetDefaultPool(ctx context.Context) (*moduleegress.Pool, error) {
	entity, err := r.client.IPv6EgressPool.Query().Where(
		dbpool.IsDefaultEQ(true),
		dbpool.StatusEQ(moduleegress.PoolStatusActive),
	).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, moduleegress.ErrPoolNotFound
		}
		return nil, err
	}
	count, err := entity.QueryBindings().Count(ctx)
	if err != nil {
		return nil, err
	}
	return poolEntityToModule(entity, int64(count)), nil
}

func (r *egressRepository) ListPools(ctx context.Context) ([]moduleegress.Pool, error) {
	entities, err := r.client.IPv6EgressPool.Query().Order(dbent.Asc(dbpool.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(entities))
	rows, err := r.sql.QueryContext(ctx, `SELECT pool_id, COUNT(*) FROM account_egress_bindings GROUP BY pool_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var poolID, count int64
		if err := rows.Scan(&poolID, &count); err != nil {
			return nil, err
		}
		counts[poolID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]moduleegress.Pool, 0, len(entities))
	for _, entity := range entities {
		out = append(out, *poolEntityToModule(entity, counts[entity.ID]))
	}
	return out, nil
}

func (r *egressRepository) GetBinding(ctx context.Context, accountID int64) (*moduleegress.Binding, error) {
	entity, err := r.client.AccountEgressBinding.Query().
		Where(dbbinding.AccountIDEQ(accountID)).
		WithAccount().
		WithPool().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, moduleegress.ErrBindingNotFound
		}
		return nil, err
	}
	return bindingEntityToModule(entity), nil
}

func (r *egressRepository) GetAnyBindingForPool(ctx context.Context, poolID int64) (*moduleegress.Binding, error) {
	entity, err := r.client.AccountEgressBinding.Query().
		Where(dbbinding.PoolIDEQ(poolID), dbbinding.StatusEQ(moduleegress.BindingStatusActive)).
		WithPool().
		Order(dbent.Asc(dbbinding.FieldAccountID)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, moduleegress.ErrBindingNotFound
		}
		return nil, err
	}
	return bindingEntityToModule(entity), nil
}

func (r *egressRepository) ListBindings(ctx context.Context, offset, limit int, search string) (*moduleegress.BindingPage, error) {
	query := r.client.AccountEgressBinding.Query()
	if search != "" {
		query = query.Where(dbbinding.Or(
			dbbinding.SourceIpv6ContainsFold(search),
			dbbinding.HasAccountWith(dbaccount.NameContainsFold(search)),
			dbbinding.HasPoolWith(dbpool.NameContainsFold(search)),
		))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	entities, err := query.
		WithAccount().
		WithPool().
		Order(dbent.Asc(dbbinding.FieldAccountID)).
		Offset(offset).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	page := &moduleegress.BindingPage{Items: make([]moduleegress.Binding, 0, len(entities)), Total: int64(total)}
	for _, entity := range entities {
		page.Items = append(page.Items, *bindingEntityToModule(entity))
	}
	return page, nil
}

func (r *egressRepository) UpsertBinding(ctx context.Context, binding moduleegress.Binding, expectedVersion *int64) (*moduleegress.Binding, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, queryErr := tx.AccountEgressBinding.Query().Where(dbbinding.AccountIDEQ(binding.AccountID)).ForUpdate().Only(ctx)
	if queryErr != nil && !dbent.IsNotFound(queryErr) {
		return nil, queryErr
	}
	var saved *dbent.AccountEgressBinding
	if current == nil {
		if expectedVersion != nil {
			return nil, moduleegress.ErrBindingChanged
		}
		saved, err = tx.AccountEgressBinding.Create().
			SetAccountID(binding.AccountID).
			SetPoolID(binding.PoolID).
			SetSourceIpv6(binding.SourceIPv6).
			SetStatus(moduleegress.BindingStatusActive).
			SetVersion(binding.Version).
			Save(ctx)
	} else {
		if expectedVersion == nil || current.Version != *expectedVersion {
			return nil, moduleegress.ErrBindingChanged
		}
		now := time.Now().UTC()
		saved, err = tx.AccountEgressBinding.UpdateOne(current).
			SetPoolID(binding.PoolID).
			SetSourceIpv6(binding.SourceIPv6).
			SetStatus(moduleegress.BindingStatusActive).
			SetVersion(binding.Version).
			SetRotatedAt(now).
			Save(ctx)
	}
	if err != nil {
		if dbent.IsConstraintError(err) && strings.Contains(strings.ToLower(err.Error()), "source_ipv6") {
			return nil, moduleegress.ErrAddressConflict
		}
		return nil, err
	}
	if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &binding.AccountID, nil, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.notifyAccount(ctx, binding.AccountID)
	return r.GetBinding(ctx, saved.AccountID)
}

func (r *egressRepository) DeleteBinding(ctx context.Context, accountID int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	deleted, err := tx.AccountEgressBinding.Delete().Where(dbbinding.AccountIDEQ(accountID)).Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return moduleegress.ErrBindingNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.notifyAccount(ctx, accountID)
	return nil
}

func (r *egressRepository) SetAccountMode(ctx context.Context, accountID int64, mode platformegress.Mode) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := tx.Account.Update().Where(dbaccount.IDEQ(accountID)).SetEgressMode(string(mode)).Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.notifyAccount(ctx, accountID)
	return nil
}

func (r *egressRepository) ListInheritedAccountIDsWithoutBinding(ctx context.Context, limit int) ([]int64, error) {
	var ids []int64
	err := r.client.Account.Query().
		Where(
			dbaccount.EgressModeEQ(string(platformegress.ModeInherit)),
			dbaccount.ProxyIDIsNil(),
			dbaccount.Not(dbaccount.HasEgressBinding()),
		).
		Order(dbent.Asc(dbaccount.FieldID)).
		Limit(limit).
		Select(dbaccount.FieldID).
		Scan(ctx, &ids)
	return ids, err
}

func (r *egressRepository) notifyAccount(ctx context.Context, accountID int64) {
	accountRepo := newAccountRepositoryWithSQL(r.client, r.sql, r.schedulerCache)
	accountRepo.syncSchedulerAccountSnapshotDetached(ctx, accountID)
}

func (r *egressRepository) notifyAccounts(ctx context.Context, accountIDs []int64) {
	if len(accountIDs) == 0 {
		return
	}
	accountRepo := newAccountRepositoryWithSQL(r.client, r.sql, r.schedulerCache)
	accountRepo.syncSchedulerAccountSnapshots(ctx, accountIDs)
}

func poolEntityToModule(entity *dbent.IPv6EgressPool, allocated int64) *moduleegress.Pool {
	if entity == nil {
		return nil
	}
	capacity, _ := moduleegress.PoolCapacity(entity.Cidr)
	return &moduleegress.Pool{
		ID:                entity.ID,
		Name:              entity.Name,
		CIDR:              entity.Cidr,
		NodeID:            entity.NodeID,
		Status:            entity.Status,
		IsDefault:         entity.IsDefault,
		AllocationVersion: entity.AllocationVersion,
		AllocatedCount:    allocated,
		Capacity:          capacity,
		CreatedAt:         entity.CreatedAt,
		UpdatedAt:         entity.UpdatedAt,
	}
}

func bindingEntityToModule(entity *dbent.AccountEgressBinding) *moduleegress.Binding {
	if entity == nil {
		return nil
	}
	out := &moduleegress.Binding{
		ID:         entity.ID,
		AccountID:  entity.AccountID,
		PoolID:     entity.PoolID,
		SourceIPv6: entity.SourceIpv6,
		Status:     entity.Status,
		Version:    entity.Version,
		RotatedAt:  entity.RotatedAt,
		CreatedAt:  entity.CreatedAt,
		UpdatedAt:  entity.UpdatedAt,
	}
	if entity.Edges.Account != nil {
		out.AccountName = entity.Edges.Account.Name
	}
	if entity.Edges.Pool != nil {
		out.PoolName = entity.Edges.Pool.Name
		out.PoolStatus = entity.Edges.Pool.Status
	}
	return out
}

var _ moduleegress.Store = (*egressRepository)(nil)
