package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

type activityCenterRepository struct {
	db *sql.DB
}

var activityCenterDB *sql.DB

func RegisterActivityCenterDB(db *sql.DB) {
	activityCenterDB = db
}

func ActivityCenterDB() *sql.DB {
	return activityCenterDB
}

func NewActivityCenterRepository(db *sql.DB) activitycenter.Repository {
	return &activityCenterRepository{db: db}
}

func (r *activityCenterRepository) Create(ctx context.Context, campaign *activitycenter.Campaign) error {
	if campaign == nil {
		return activitycenter.ErrCampaignInputRequired
	}

	row := r.db.QueryRowContext(ctx, `
INSERT INTO act_campaigns (
	title, subtitle, banner_url, type, ref_id, status, starts_at, ends_at,
	sort_order, content, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at, updated_at
`,
		campaign.Title,
		campaign.Subtitle,
		campaign.BannerURL,
		campaign.Type,
		campaign.RefID,
		campaign.Status,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.SortOrder,
		campaign.Content,
		campaign.CreatedBy,
	)
	if err := row.Scan(&campaign.ID, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
		return fmt.Errorf("insert activity campaign: %w", err)
	}
	return nil
}

func (r *activityCenterRepository) GetByID(ctx context.Context, id int64) (*activitycenter.Campaign, error) {
	return r.getOne(ctx, "WHERE id = $1 AND deleted_at IS NULL", id)
}

func (r *activityCenterRepository) GetVisibleByID(ctx context.Context, id int64, now time.Time) (*activitycenter.Campaign, error) {
	return r.getOne(ctx, `
WHERE id = $1
  AND deleted_at IS NULL
  AND status = 'active'
  AND (starts_at IS NULL OR starts_at <= $2)
  AND (ends_at IS NULL OR ends_at > $2)
`, id, now)
}

func (r *activityCenterRepository) Update(ctx context.Context, campaign *activitycenter.Campaign) error {
	if campaign == nil {
		return activitycenter.ErrCampaignInputRequired
	}

	row := r.db.QueryRowContext(ctx, `
UPDATE act_campaigns
SET title = $2,
    subtitle = $3,
    banner_url = $4,
    type = $5,
    ref_id = $6,
    status = $7,
    starts_at = $8,
    ends_at = $9,
    sort_order = $10,
    content = $11,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING updated_at
`,
		campaign.ID,
		campaign.Title,
		campaign.Subtitle,
		campaign.BannerURL,
		campaign.Type,
		campaign.RefID,
		campaign.Status,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.SortOrder,
		campaign.Content,
	)
	if err := row.Scan(&campaign.UpdatedAt); err != nil {
		return translatePersistenceError(err, activitycenter.ErrCampaignNotFound, nil)
	}
	return nil
}

func (r *activityCenterRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE act_campaigns
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return activitycenter.ErrCampaignNotFound
	}
	return nil
}

func (r *activityCenterRepository) List(
	ctx context.Context,
	params pagination.PaginationParams,
	filters activitycenter.ListFilters,
) ([]activitycenter.Campaign, *pagination.PaginationResult, error) {
	where, args := activityCampaignWhere(filters)
	totalQuery := "SELECT COUNT(*) FROM act_campaigns " + where

	var total int64
	if err := r.db.QueryRowContext(ctx, totalQuery, args...).Scan(&total); err != nil {
		return nil, nil, err
	}

	orderBy := activityCampaignOrderBy(params)
	args = append(args, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT id, title, subtitle, banner_url, type, ref_id, status, starts_at, ends_at,
       sort_order, content, created_by, created_at, updated_at
FROM act_campaigns
`+where+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args))+`
`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items, err := scanActivityCampaignRows(rows)
	if err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *activityCenterRepository) ListVisible(ctx context.Context, now time.Time) ([]activitycenter.Campaign, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, title, subtitle, banner_url, type, ref_id, status, starts_at, ends_at,
       sort_order, content, created_by, created_at, updated_at
FROM act_campaigns
WHERE deleted_at IS NULL
  AND status = 'active'
  AND (starts_at IS NULL OR starts_at <= $1)
  AND (ends_at IS NULL OR ends_at > $1)
ORDER BY sort_order ASC, COALESCE(starts_at, created_at) DESC, id DESC
LIMIT 200
`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityCampaignRows(rows)
}

func (r *activityCenterRepository) getOne(ctx context.Context, where string, args ...any) (*activitycenter.Campaign, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, title, subtitle, banner_url, type, ref_id, status, starts_at, ends_at,
       sort_order, content, created_by, created_at, updated_at
FROM act_campaigns
`+where+`
LIMIT 1
`, args...)

	item, err := scanActivityCampaign(row)
	if err != nil {
		return nil, translatePersistenceError(err, activitycenter.ErrCampaignNotFound, nil)
	}
	return item, nil
}

func activityCampaignWhere(filters activitycenter.ListFilters) (string, []any) {
	clauses := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 3)
	if filters.Status != "" {
		args = append(args, filters.Status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filters.Type != "" {
		args = append(args, filters.Type)
		clauses = append(clauses, fmt.Sprintf("type = $%d", len(args)))
	}
	if filters.Search != "" {
		args = append(args, "%"+filters.Search+"%")
		clauses = append(clauses, fmt.Sprintf("(title ILIKE $%d OR subtitle ILIKE $%d OR content ILIKE $%d)", len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func activityCampaignOrderBy(params pagination.PaginationParams) string {
	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderDesc))
	switch strings.ToLower(strings.TrimSpace(params.SortBy)) {
	case "title":
		return " ORDER BY title " + sortOrder + ", id " + sortOrder
	case "status":
		return " ORDER BY status " + sortOrder + ", id " + sortOrder
	case "type":
		return " ORDER BY type " + sortOrder + ", id " + sortOrder
	case "sort_order":
		return " ORDER BY sort_order " + sortOrder + ", id DESC"
	case "starts_at":
		return " ORDER BY starts_at " + sortOrder + " NULLS LAST, id DESC"
	case "ends_at":
		return " ORDER BY ends_at " + sortOrder + " NULLS LAST, id DESC"
	case "id":
		return " ORDER BY id " + sortOrder
	default:
		return " ORDER BY created_at " + sortOrder + ", id " + sortOrder
	}
}

type activityCampaignScanner interface {
	Scan(dest ...any) error
}

func scanActivityCampaignRows(rows *sql.Rows) ([]activitycenter.Campaign, error) {
	items := make([]activitycenter.Campaign, 0)
	for rows.Next() {
		item, err := scanActivityCampaign(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanActivityCampaign(scanner activityCampaignScanner) (*activitycenter.Campaign, error) {
	var item activitycenter.Campaign
	var startsAt sql.NullTime
	var endsAt sql.NullTime
	var createdBy sql.NullInt64
	err := scanner.Scan(
		&item.ID,
		&item.Title,
		&item.Subtitle,
		&item.BannerURL,
		&item.Type,
		&item.RefID,
		&item.Status,
		&startsAt,
		&endsAt,
		&item.SortOrder,
		&item.Content,
		&createdBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if startsAt.Valid {
		item.StartsAt = &startsAt.Time
	}
	if endsAt.Valid {
		item.EndsAt = &endsAt.Time
	}
	if createdBy.Valid {
		item.CreatedBy = &createdBy.Int64
	}
	return &item, nil
}
