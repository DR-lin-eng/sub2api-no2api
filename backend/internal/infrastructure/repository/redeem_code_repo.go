package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeusage"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"

	entsql "entgo.io/ent/dialect/sql"
)

type redeemCodeRepository struct {
	client *dbent.Client
}

func NewRedeemCodeRepository(client *dbent.Client) service.RedeemCodeRepository {
	return &redeemCodeRepository{client: client}
}

func (r *redeemCodeRepository) Create(ctx context.Context, code *service.RedeemCode) error {
	if !code.LimitsConfigured && code.MaxUses == 0 && code.MaxUsesPerUser == 0 {
		code.MaxUses = 1
		code.MaxUsesPerUser = 1
	}
	created, err := r.client.RedeemCode.Create().
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetMaxUses(code.MaxUses).
		SetUsedCount(code.UsedCount).
		SetMaxUsesPerUser(code.MaxUsesPerUser).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays).
		SetNillableExpiresAt(code.ExpiresAt).
		SetNillableUsedBy(code.UsedBy).
		SetNillableUsedAt(code.UsedAt).
		SetNillableGroupID(code.GroupID).
		Save(ctx)
	if err == nil {
		code.ID = created.ID
		code.CreatedAt = created.CreatedAt
	}
	return err
}

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	if len(codes) == 0 {
		return nil
	}

	builders := make([]*dbent.RedeemCodeCreate, 0, len(codes))
	for i := range codes {
		c := &codes[i]
		if !c.LimitsConfigured && c.MaxUses == 0 && c.MaxUsesPerUser == 0 {
			c.MaxUses = 1
			c.MaxUsesPerUser = 1
		}
		b := r.client.RedeemCode.Create().
			SetCode(c.Code).
			SetType(c.Type).
			SetValue(c.Value).
			SetStatus(c.Status).
			SetMaxUses(c.MaxUses).
			SetUsedCount(c.UsedCount).
			SetMaxUsesPerUser(c.MaxUsesPerUser).
			SetNotes(c.Notes).
			SetValidityDays(c.ValidityDays).
			SetNillableExpiresAt(c.ExpiresAt).
			SetNillableUsedBy(c.UsedBy).
			SetNillableUsedAt(c.UsedAt).
			SetNillableGroupID(c.GroupID)
		builders = append(builders, b)
	}

	return r.client.RedeemCode.CreateBulk(builders...).Exec(ctx)
}

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.RedeemCode.Delete().Where(redeemcode.IDEQ(id)).Exec(ctx)
	return err
}

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
}

func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := r.client.RedeemCode.Query()

	if codeType != "" {
		q = q.Where(redeemcode.TypeEQ(codeType))
	}
	if status != "" {
		now := time.Now()
		switch status {
		case service.StatusExpired:
			q = q.Where(redeemcode.Or(
				redeemcode.StatusEQ(service.StatusExpired),
				redeemcode.And(
					redeemcode.StatusEQ(service.StatusUnused),
					redeemcode.ExpiresAtNotNil(),
					redeemcode.ExpiresAtLTE(now),
				),
			))
		case service.StatusUnused:
			q = q.Where(
				redeemcode.StatusEQ(service.StatusUnused),
				redeemcode.Or(
					redeemcode.ExpiresAtIsNil(),
					redeemcode.ExpiresAtGT(now),
				),
			)
		default:
			q = q.Where(redeemcode.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(
			redeemcode.Or(
				redeemcode.CodeContainsFold(search),
				redeemcode.HasUserWith(user.EmailContainsFold(search)),
			),
		)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	codesQuery := q.
		WithUser().
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range redeemCodeListOrder(params) {
		codesQuery = codesQuery.Order(order)
	}

	codes, err := codesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outCodes := redeemCodeEntitiesToService(codes)

	return outCodes, paginationResultFromTotal(int64(total), params), nil
}

func redeemCodeListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	switch sortBy {
	case "type":
		field = redeemcode.FieldType
	case "value":
		field = redeemcode.FieldValue
	case "status":
		field = redeemcode.FieldStatus
	case "used_at":
		field = redeemcode.FieldUsedAt
	case "created_at":
		field = redeemcode.FieldCreatedAt
	case "expires_at":
		field = redeemcode.FieldExpiresAt
	case "code":
		field = redeemcode.FieldCode
	default:
		field = redeemcode.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(redeemcode.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(redeemcode.FieldID)}
}

func (r *redeemCodeRepository) Update(ctx context.Context, code *service.RedeemCode) error {
	if !code.LimitsConfigured && code.MaxUses == 0 && code.MaxUsesPerUser == 0 {
		code.MaxUses = 1
		code.MaxUsesPerUser = 1
	}
	up := r.client.RedeemCode.UpdateOneID(code.ID).
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetMaxUses(code.MaxUses).
		SetUsedCount(code.UsedCount).
		SetMaxUsesPerUser(code.MaxUsesPerUser).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays)

	if code.UsedBy != nil {
		up.SetUsedBy(*code.UsedBy)
	} else {
		up.ClearUsedBy()
	}
	if code.UsedAt != nil {
		up.SetUsedAt(*code.UsedAt)
	} else {
		up.ClearUsedAt()
	}
	if code.GroupID != nil {
		up.SetGroupID(*code.GroupID)
	} else {
		up.ClearGroupID()
	}
	if code.ExpiresAt != nil {
		up.SetExpiresAt(*code.ExpiresAt)
	} else {
		up.ClearExpiresAt()
	}

	updated, err := up.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrRedeemCodeNotFound
		}
		return err
	}
	code.CreatedAt = updated.CreatedAt
	return nil
}

func (r *redeemCodeRepository) BatchUpdate(ctx context.Context, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return 0, nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.batchUpdate(ctx, tx.Client(), uniqueIDs, fields)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()

	updated, err := r.batchUpdate(txCtx, tx.Client(), uniqueIDs, fields)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func (r *redeemCodeRepository) batchUpdate(ctx context.Context, client *dbent.Client, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	existing, err := client.RedeemCode.Query().
		Where(redeemcode.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	if fields.TouchesUsedSensitiveFields() {
		for _, code := range existing {
			if code.Status == service.StatusUsed {
				return 0, service.ErrRedeemCodeUsed
			}
		}
	}

	up := client.RedeemCode.Update().Where(redeemcode.IDIn(ids...))
	if fields.Status != nil {
		up.SetStatus(*fields.Status)
	}
	if fields.Notes != nil {
		up.SetNotes(*fields.Notes)
	}
	if fields.ExpiresAt.Set {
		if fields.ExpiresAt.Value != nil {
			up.SetExpiresAt(*fields.ExpiresAt.Value)
		} else {
			up.ClearExpiresAt()
		}
	}
	if fields.GroupID.Set {
		if fields.GroupID.Value != nil {
			up.SetGroupID(*fields.GroupID.Value)
		} else {
			up.ClearGroupID()
		}
	}

	affected, err := up.Save(ctx)
	if err != nil {
		return 0, err
	}
	if affected != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	return int64(affected), nil
}

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	client := clientFromContext(ctx, r.client)
	code, err := client.RedeemCode.Query().Where(redeemcode.IDEQ(id)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrRedeemCodeNotFound
		}
		return err
	}
	if code.Status != service.StatusUnused || (code.MaxUses > 0 && code.UsedCount >= code.MaxUses) {
		return service.ErrRedeemCodeUsed
	}
	if code.MaxUsesPerUser > 0 {
		usedCount, err := client.RedeemCodeUsage.Query().Where(
			redeemcodeusage.RedeemCodeIDEQ(id),
			redeemcodeusage.UserIDEQ(userID),
		).Count(ctx)
		if err != nil {
			return err
		}
		if usedCount >= code.MaxUsesPerUser {
			return service.ErrRedeemCodeUsed
		}
	}
	now := time.Now()
	newCount := code.UsedCount + 1
	status := service.StatusUnused
	if code.MaxUses > 0 && newCount >= code.MaxUses {
		status = service.StatusUsed
	}
	if _, err := client.RedeemCode.UpdateOneID(id).
		SetStatus(status).
		SetUsedCount(newCount).
		SetUsedBy(userID).
		SetUsedAt(now).
		Save(ctx); err != nil {
		return err
	}
	if _, err := client.RedeemCodeUsage.Create().
		SetRedeemCodeID(id).
		SetUserID(userID).
		SetValue(code.Value).
		SetUsedAt(now).
		Save(ctx); err != nil {
		return err
	}
	return nil
}

func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 10
	}

	usages, err := r.client.RedeemCodeUsage.Query().
		Where(redeemcodeusage.UserIDEQ(userID)).
		WithRedeemCode(func(q *dbent.RedeemCodeQuery) { q.WithGroup() }).
		Order(dbent.Desc(redeemcodeusage.FieldUsedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.RedeemCode, 0, len(usages))
	seen := make(map[int64]struct{}, len(usages))
	for _, usage := range usages {
		code := redeemCodeEntityToService(usage.Edges.RedeemCode)
		if code == nil {
			continue
		}
		code.UsedBy = &userID
		code.UsedAt = &usage.UsedAt
		out = append(out, *code)
		seen[code.ID] = struct{}{}
	}
	// Legacy/admin adjustment records do not have a usage row.
	legacy, err := r.client.RedeemCode.Query().Where(redeemcode.UsedByEQ(userID)).WithGroup().Order(dbent.Desc(redeemcode.FieldUsedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, code := range legacy {
		if _, ok := seen[code.ID]; ok {
			continue
		}
		out = append(out, *redeemCodeEntityToService(code))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UsedAt == nil {
			return false
		}
		if out[j].UsedAt == nil {
			return true
		}
		return out[i].UsedAt.After(*out[j].UsedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListByUserPaginated returns paginated balance/concurrency history for a user.
// Supports optional type filter (e.g. "balance", "admin_balance", "concurrency", "admin_concurrency", "subscription").
func (r *redeemCodeRepository) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	usages, err := r.client.RedeemCodeUsage.Query().Where(redeemcodeusage.UserIDEQ(userID)).WithRedeemCode(func(q *dbent.RedeemCodeQuery) { q.WithGroup() }).Order(dbent.Desc(redeemcodeusage.FieldUsedAt)).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	all := make([]service.RedeemCode, 0, len(usages))
	seen := make(map[int64]struct{}, len(usages))
	for _, usage := range usages {
		code := redeemCodeEntityToService(usage.Edges.RedeemCode)
		if code == nil || (codeType != "" && code.Type != codeType) {
			continue
		}
		code.UsedBy = &userID
		code.UsedAt = &usage.UsedAt
		all = append(all, *code)
		seen[code.ID] = struct{}{}
	}
	legacy, err := r.client.RedeemCode.Query().Where(redeemcode.UsedByEQ(userID)).WithGroup().Order(dbent.Desc(redeemcode.FieldUsedAt)).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, model := range legacy {
		if _, ok := seen[model.ID]; ok || (codeType != "" && model.Type != codeType) {
			continue
		}
		all = append(all, *redeemCodeEntityToService(model))
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].UsedAt == nil {
			return false
		}
		if all[j].UsedAt == nil {
			return true
		}
		return all[i].UsedAt.After(*all[j].UsedAt)
	})
	total := len(all)
	start := params.Offset()
	if start > total {
		start = total
	}
	end := start + params.Limit()
	if end > total {
		end = total
	}
	return all[start:end], paginationResultFromTotal(int64(total), params), nil
}

// SumPositiveBalanceByUser returns total recharged amount (sum of value > 0 where type is balance/admin_balance).
func (r *redeemCodeRepository) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	usages, err := r.client.RedeemCodeUsage.Query().Where(redeemcodeusage.UserIDEQ(userID)).WithRedeemCode().All(ctx)
	if err != nil {
		return 0, err
	}
	seen := make(map[int64]struct{}, len(usages))
	var total float64
	for _, usage := range usages {
		code := usage.Edges.RedeemCode
		if code != nil {
			seen[code.ID] = struct{}{}
			if code.Value > 0 && (code.Type == "balance" || code.Type == "admin_balance") {
				total += code.Value
			}
		}
	}
	legacy, err := r.client.RedeemCode.Query().
		Where(
			redeemcode.UsedByEQ(userID),
			redeemcode.ValueGT(0),
			redeemcode.TypeIn("balance", "admin_balance"),
		).All(ctx)
	if err != nil {
		return 0, err
	}
	for _, code := range legacy {
		if _, ok := seen[code.ID]; !ok {
			total += code.Value
		}
	}
	return total, nil
}

func redeemCodeEntityToService(m *dbent.RedeemCode) *service.RedeemCode {
	if m == nil {
		return nil
	}
	out := &service.RedeemCode{
		ID:               m.ID,
		Code:             m.Code,
		Type:             m.Type,
		Value:            m.Value,
		Status:           m.Status,
		MaxUses:          m.MaxUses,
		UsedCount:        m.UsedCount,
		MaxUsesPerUser:   m.MaxUsesPerUser,
		LimitsConfigured: true,
		UsedBy:           m.UsedBy,
		UsedAt:           m.UsedAt,
		Notes:            derefString(m.Notes),
		CreatedAt:        m.CreatedAt,
		ExpiresAt:        m.ExpiresAt,
		GroupID:          m.GroupID,
		ValidityDays:     m.ValidityDays,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	return out
}

func redeemCodeEntitiesToService(models []*dbent.RedeemCode) []service.RedeemCode {
	out := make([]service.RedeemCode, 0, len(models))
	for i := range models {
		if s := redeemCodeEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}
