package repository

import (
	"context"
	"encoding/json"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

func (r *accountRepository) Create(ctx context.Context, account *service.Account) error {
	if err := createAccountRecord(ctx, r.client, account); err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account create failed: account=%d err=%v", account.ID, err)
	}
	return nil
}

func createAccountRecord(ctx context.Context, client *dbent.Client, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
	}

	builder := client.Account.Create().
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	if account.ParentAccountID != nil {
		builder.SetParentAccountID(*account.ParentAccountID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
}

// CreateWithAccountGroups atomically persists an account, its exact per-group priorities,
// and the scheduler outbox event used to publish the new routing snapshot.
func (r *accountRepository) CreateWithAccountGroups(ctx context.Context, account *service.Account, groups []service.AccountGroup) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// Reuse a caller-owned transaction when this repository is already transactional.
		txClient = r.client
	}

	if err := createAccountRecord(ctx, txClient, account); err != nil {
		return err
	}
	groupIDs := make([]int64, 0, len(groups))
	if len(groups) > 0 {
		builders := make([]*dbent.AccountGroupCreate, 0, len(groups))
		for i := range groups {
			groups[i].AccountID = account.ID
			groupIDs = append(groupIDs, groups[i].GroupID)
			builders = append(builders, txClient.AccountGroup.Create().
				SetAccountID(account.ID).
				SetGroupID(groups[i].GroupID).
				SetPriority(groups[i].Priority),
			)
		}
		if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}
	account.GroupIDs = groupIDs
	account.AccountGroups = append([]service.AccountGroup(nil), groups...)
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	m, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, service.ErrAccountNotFound
	}
	return &accounts[0], nil
}

func (r *accountRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if len(ids) == 0 {
		return []*service.Account{}, nil
	}

	// De-duplicate while preserving order of first occurrence.
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}

	entAccounts, err := r.client.Account.
		Query().
		Where(dbaccount.IDIn(uniqueIDs...)).
		WithProxy().
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entAccounts) == 0 {
		return []*service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(entAccounts))
	entByID := make(map[int64]*dbent.Account, len(entAccounts))
	for _, acc := range entAccounts {
		entByID[acc.ID] = acc
		accountIDs = append(accountIDs, acc.ID)
	}

	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outByID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAcc := range entAccounts {
		out := accountEntityToService(entAcc)
		if out == nil {
			continue
		}

		// Prefer the preloaded proxy edge when available.
		if entAcc.Edges.Proxy != nil {
			out.Proxy = proxyEntityToService(entAcc.Edges.Proxy)
		}

		if groups, ok := groupsByAccount[entAcc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[entAcc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[entAcc.ID]; ok {
			out.AccountGroups = ags
		}
		outByID[entAcc.ID] = out
	}

	// Preserve input order (first occurrence), and ignore missing IDs.
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := entByID[id]; !ok {
			continue
		}
		if acc, ok := outByID[id]; ok && acc != nil {
			out = append(out, acc)
		}
	}

	return out, nil
}

// ExistsByID 检查指定 ID 的账号是否存在。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Exist() 方法生成 SELECT EXISTS 查询，只返回布尔值
//   - 不加载完整的账号实体及其关联数据（Groups、Proxy 等）
//   - 适用于删除前的存在性检查等只需判断有无的场景
func (r *accountRepository) ExistsByID(ctx context.Context, id int64) (bool, error) {
	exists, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	if crsAccountID == "" {
		return nil, nil
	}

	// 使用 sqljson.ValueEQ 生成 JSON 路径过滤，避免手写 SQL 片段导致语法兼容问题。
	// 排除 spark 影子账号(parent_account_id 非空):影子不持凭据,绝不能被 CRS 当作普通账号
	// 更新而覆盖 type/credentials/proxy。即便影子 Extra 被误写入 crs_account_id 也不会命中
	// (外审第7轮 P1)。
	m, err := r.client.Account.Query().
		Where(dbaccount.ParentAccountIDIsNil()).
		Where(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, crsAccountID, sqljson.Path("crs_account_id")))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return &accounts[0], nil
}

func (r *accountRepository) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	// parent_account_id IS NULL 排除 spark 影子账号:影子不是 CRS 账号,绝不能进 CRS 同步映射
	// (否则会被当普通账号更新而覆盖 type/credentials/proxy)(外审第7轮 P1)。
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, extra->>'crs_account_id'
		FROM accounts
		WHERE deleted_at IS NULL
			AND parent_account_id IS NULL
			AND extra->>'crs_account_id' IS NOT NULL
			AND extra->>'crs_account_id' != ''
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int64)
	for rows.Next() {
		var id int64
		var crsID string
		if err := rows.Scan(&id, &crsID); err != nil {
			return nil, err
		}
		result[crsID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountRepository) Update(ctx context.Context, account *service.Account) error {
	return r.updateAccount(ctx, account, nil)
}

// UpdateWithUpstreamBillingProbeEnabled applies an explicit probe switch in the
// same row-lock transaction as the rest of an admin account edit.
func (r *accountRepository) UpdateWithUpstreamBillingProbeEnabled(ctx context.Context, account *service.Account, enabled bool) error {
	return r.updateAccount(ctx, account, &enabled)
}

func (r *accountRepository) updateAccount(ctx context.Context, account *service.Account, explicitProbeEnabled *bool) error {
	if account == nil {
		return nil
	}

	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}

	updated, err := r.updateLockedAccount(ctx, client, account, explicitProbeEnabled)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	account.UpdatedAt = updated.UpdatedAt
	// 普通账号编辑（如 model_mapping / credentials）也需要立即刷新单账号快照，
	// 否则网关在 outbox worker 延迟或异常时仍可能读到旧配置。
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, account.ID)
	}
	return nil
}

func (r *accountRepository) updateLockedAccount(ctx context.Context, client *dbent.Client, account *service.Account, explicitProbeEnabled *bool) (*dbent.Account, error) {
	extra, err := lockAndMergeAccountProbeExtra(ctx, client, account, explicitProbeEnabled)
	if err != nil {
		return nil, err
	}
	account.Extra = extra

	schedulable := account.Schedulable
	if account.Status == service.StatusError {
		schedulable = false
	}

	builder := client.Account.UpdateOneID(account.ID).
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(extra).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	} else {
		builder.ClearLoadFactor()
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	} else {
		builder.ClearProxyID()
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	} else {
		builder.ClearLastUsedAt()
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	} else {
		builder.ClearRateLimitedAt()
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	} else {
		builder.ClearRateLimitResetAt()
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	} else {
		builder.ClearOverloadUntil()
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	} else {
		builder.ClearSessionWindowStart()
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	} else {
		builder.ClearSessionWindowEnd()
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	} else {
		builder.ClearSessionWindowStatus()
	}
	if account.Notes == nil {
		builder.ClearNotes()
	}

	builder.SetQuotaDimension(dbaccount.QuotaDimension(account.QuotaDimensionOrDefault()))
	builder.SetNillableParentAccountID(account.ParentAccountID)

	return builder.Save(ctx)
}

func lockAndMergeAccountProbeExtra(ctx context.Context, client *dbent.Client, account *service.Account, explicitProbeEnabled *bool) (map[string]any, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return nil, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	rows, err := client.QueryContext(ctx, `
		SELECT
			platform = $2
			AND type = $3
			AND credentials = $4::jsonb
			AND proxy_id IS NOT DISTINCT FROM $5,
			extra -> 'upstream_billing_probe_enabled',
			extra -> 'upstream_billing_probe'
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}

	var (
		identityUnchanged bool
		currentEnabled    []byte
		currentSnapshot   []byte
	)
	if err := rows.Scan(&identityUnchanged, &currentEnabled, &currentSnapshot); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	extra := copyJSONMap(normalizeJSONMap(account.Extra))
	delete(extra, service.UpstreamBillingProbeEnabledExtraKey)
	delete(extra, service.UpstreamBillingProbeExtraKey)
	probeExplicitlyDisabled := false
	probeAccount := account.Platform == service.PlatformOpenAI && account.Type == service.AccountTypeAPIKey
	if probeAccount && explicitProbeEnabled != nil {
		extra[service.UpstreamBillingProbeEnabledExtraKey] = *explicitProbeEnabled
		probeExplicitlyDisabled = !*explicitProbeEnabled
	} else if probeAccount && len(currentEnabled) > 0 && string(currentEnabled) != "null" {
		var enabled any
		if err := json.Unmarshal(currentEnabled, &enabled); err != nil {
			return nil, err
		}
		extra[service.UpstreamBillingProbeEnabledExtraKey] = enabled
		if value, ok := enabled.(bool); ok && !value {
			probeExplicitlyDisabled = true
		}
	}
	if !identityUnchanged || probeExplicitlyDisabled || len(currentSnapshot) == 0 || string(currentSnapshot) == "null" {
		return extra, nil
	}
	var snapshot any
	if err := json.Unmarshal(currentSnapshot, &snapshot); err != nil {
		return nil, err
	}
	extra[service.UpstreamBillingProbeExtraKey] = snapshot
	return extra, nil
}

func (r *accountRepository) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	payload, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return err
	}
	baseCtx := ctx
	contextTx := dbent.TxFromContext(ctx)
	client := r.client
	var tx *dbent.Tx
	if contextTx != nil {
		client = contextTx.Client()
	} else if r.client != nil {
		var txErr error
		tx, txErr = r.client.Tx(ctx)
		if txErr != nil && !errors.Is(txErr, dbent.ErrTxStarted) {
			return txErr
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			ctx = dbent.NewTxContext(ctx, tx)
			client = tx.Client()
		}
	}
	result, err := client.ExecContext(ctx, `
		UPDATE accounts
		SET
			credentials = $1::jsonb,
			extra = CASE
				WHEN platform = 'openai'
					AND type = 'apikey'
					AND credentials IS DISTINCT FROM $1::jsonb
				THEN COALESCE(extra, '{}'::jsonb) - 'upstream_billing_probe'
				ELSE extra
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, string(payload), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if contextTx == nil {
		r.syncSchedulerAccountSnapshot(baseCtx, id)
	}
	return nil
}

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	groupIDs, err := r.loadAccountGroupIDs(ctx, id)
	if err != nil {
		return err
	}
	// 使用事务保证账号与关联分组的删除原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if _, err := txClient.ExecContext(ctx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", id); err != nil {
		return err
	}
	if _, err := txClient.Account.Delete().Where(dbaccount.IDEQ(id)).Exec(ctx); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	r.deleteSchedulerAccountSnapshot(ctx, id)
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, buildSchedulerGroupPayload(groupIDs)); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account delete failed: account=%d err=%v", id, err)
	}
	return nil
}
