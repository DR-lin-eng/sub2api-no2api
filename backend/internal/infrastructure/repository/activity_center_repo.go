package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

type activityCenterRepository struct {
	db     *sql.DB
	client *dbent.Client
}

type activityCenterSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func NewActivityCenterRepository(db *sql.DB, client *dbent.Client) activitycenter.Repository {
	return &activityCenterRepository{db: db, client: client}
}

func (r *activityCenterRepository) executor(ctx context.Context) activityCenterSQLExecutor {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.db
}

func (r *activityCenterRepository) queryRow(ctx context.Context, query string, args ...any) activityCampaignScanner {
	rows, err := r.executor(ctx).QueryContext(ctx, query, args...)
	return &activityCenterQueryRow{rows: rows, err: err}
}

func (r *activityCenterRepository) WithParticipationTx(ctx context.Context, campaignID int64, fn func(context.Context) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		if _, err := tx.Client().ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('activity-campaign:' || $1::text, 0))`, campaignID); err != nil {
			return fmt.Errorf("lock activity campaign: %w", err)
		}
		return fn(ctx)
	}
	if r.client == nil {
		return fmt.Errorf("activity center transaction client is unavailable")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin activity participation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err := tx.Client().ExecContext(txCtx, `SELECT pg_advisory_xact_lock(hashtextextended('activity-campaign:' || $1::text, 0))`, campaignID); err != nil {
		return fmt.Errorf("lock activity campaign: %w", err)
	}
	if err := fn(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit activity participation transaction: %w", err)
	}
	return nil
}

func (r *activityCenterRepository) Create(ctx context.Context, campaign *activitycenter.Campaign) error {
	if campaign == nil {
		return activitycenter.ErrCampaignInputRequired
	}

	row := r.queryRow(ctx, `
INSERT INTO act_campaigns (
	title, subtitle, banner_url, banner_html, type, ref_id, status, starts_at, ends_at,
	sort_order, content, config_json, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, created_at, updated_at
`,
		campaign.Title,
		campaign.Subtitle,
		campaign.BannerURL,
		campaign.BannerHTML,
		campaign.Type,
		campaign.RefID,
		campaign.Status,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.SortOrder,
		campaign.Content,
		campaign.ConfigJSON,
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

	row := r.queryRow(ctx, `
UPDATE act_campaigns
SET title = $2,
    subtitle = $3,
    banner_url = $4,
    banner_html = $5,
    type = $6,
    ref_id = $7,
    status = $8,
    starts_at = $9,
    ends_at = $10,
    sort_order = $11,
    content = $12,
    config_json = $13,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING updated_at
`,
		campaign.ID,
		campaign.Title,
		campaign.Subtitle,
		campaign.BannerURL,
		campaign.BannerHTML,
		campaign.Type,
		campaign.RefID,
		campaign.Status,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.SortOrder,
		campaign.Content,
		campaign.ConfigJSON,
	)
	if err := row.Scan(&campaign.UpdatedAt); err != nil {
		return translatePersistenceError(err, activitycenter.ErrCampaignNotFound, nil)
	}
	return nil
}

func (r *activityCenterRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.executor(ctx).ExecContext(ctx, `
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
	if err := r.queryRow(ctx, totalQuery, args...).Scan(&total); err != nil {
		return nil, nil, err
	}

	orderBy := activityCampaignOrderBy(params)
	args = append(args, params.Limit(), params.Offset())
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT id, title, subtitle, banner_url, banner_html, type, ref_id, config_json, status, starts_at, ends_at,
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
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT id, title, subtitle, banner_url, banner_html, type, ref_id, config_json, status, starts_at, ends_at,
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

func (r *activityCenterRepository) CreateRecord(ctx context.Context, record *activitycenter.Record) error {
	if record == nil {
		return activitycenter.ErrCampaignInputRequired
	}
	row := r.queryRow(ctx, `
INSERT INTO act_participation_records (
	campaign_id, campaign_title, campaign_type, user_id,
	pool_id, pool_name, prize_id, prize_label, prize_type, prize_color,
	result_status, reward_status, reward_payload_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, created_at
`,
		record.CampaignID,
		record.CampaignTitle,
		record.CampaignType,
		record.UserID,
		record.PoolID,
		record.PoolName,
		record.PrizeID,
		record.PrizeLabel,
		record.PrizeType,
		record.PrizeColor,
		record.ResultStatus,
		record.RewardStatus,
		record.RewardPayloadJSON,
	)
	if err := row.Scan(&record.ID, &record.CreatedAt); err != nil {
		return fmt.Errorf("insert activity participation record: %w", err)
	}
	return nil
}

func (r *activityCenterRepository) GetCheckinStatus(ctx context.Context, campaignID, userID int64, checkinDate time.Time, cycleDays int) (*activitycenter.CheckinStatus, error) {
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT id, campaign_id, user_id, checkin_date, cycle_no, cycle_day, streak_days,
       reward_type, reward_value, reward_status, reward_payload_json, created_at
FROM act_checkin_records
WHERE campaign_id = $1 AND user_id = $2 AND checkin_date >= $3::date - ($4::int * INTERVAL '1 day')
ORDER BY checkin_date DESC, id DESC LIMIT $4`, campaignID, userID, checkinDate.Format("2006-01-02"), cycleDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	status := &activitycenter.CheckinStatus{}
	for rows.Next() {
		var item activitycenter.CheckinRecord
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.UserID, &item.CheckinDate, &item.CycleNo, &item.CycleDay, &item.StreakDays, &item.RewardType, &item.RewardValue, &item.RewardStatus, &item.RewardPayloadJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		status.Records = append(status.Records, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(status.Records) == 0 {
		return status, nil
	}
	latest := status.Records[0]
	status.LastCheckinDate = &latest.CheckinDate
	latestDate := latest.CheckinDate.Format("2006-01-02")
	today := checkinDate.Format("2006-01-02")
	status.CheckedToday = latestDate == today
	if status.CheckedToday || latestDate == checkinDate.AddDate(0, 0, -1).Format("2006-01-02") {
		status.StreakDays = latest.StreakDays
		status.CycleDay = latest.CycleDay
	}
	return status, nil
}

func (r *activityCenterRepository) CreateCheckinRecord(ctx context.Context, record *activitycenter.CheckinRecord) error {
	if record == nil {
		return activitycenter.ErrCampaignInputRequired
	}
	row := r.queryRow(ctx, `
INSERT INTO act_checkin_records (campaign_id, user_id, checkin_date, cycle_no, cycle_day, streak_days, reward_type, reward_value, reward_status, reward_payload_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, created_at`, record.CampaignID, record.UserID, record.CheckinDate, record.CycleNo, record.CycleDay, record.StreakDays, record.RewardType, record.RewardValue, record.RewardStatus, record.RewardPayloadJSON)
	if err := row.Scan(&record.ID, &record.CreatedAt); err != nil {
		return translatePersistenceError(err, activitycenter.ErrCampaignAlreadyCheckedIn, nil)
	}
	return nil
}

func (r *activityCenterRepository) ListCheckinLeaderboard(ctx context.Context, campaignID int64, limit int) ([]activitycenter.CheckinLeaderboardEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT COALESCE(NULLIF(BTRIM(u.username), ''), ''), COALESCE(u.email, ''), MAX(r.streak_days), COUNT(*)
FROM act_checkin_records r
LEFT JOIN users u ON u.id = r.user_id
WHERE r.campaign_id = $1
GROUP BY r.user_id, COALESCE(NULLIF(BTRIM(u.username), ''), ''), COALESCE(u.email, '')
ORDER BY MAX(r.streak_days) DESC, COUNT(*) DESC, MAX(r.checkin_date) DESC
LIMIT $2`, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]activitycenter.CheckinLeaderboardEntry, 0, limit)
	for rows.Next() {
		var item activitycenter.CheckinLeaderboardEntry
		if err := rows.Scan(&item.UserName, &item.UserEmail, &item.StreakDays, &item.CheckinCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *activityCenterRepository) CountUserPoolRecordsSince(ctx context.Context, userID, campaignID int64, poolID string, since time.Time) (int64, error) {
	var count int64
	err := r.queryRow(ctx, `
SELECT COUNT(*)
FROM act_participation_records
WHERE user_id = $1 AND campaign_id = $2 AND pool_id = $3 AND created_at >= $4
`, userID, campaignID, poolID, since).Scan(&count)
	return count, err
}

func (r *activityCenterRepository) CountPrizeRecords(ctx context.Context, campaignID int64, prizeID string) (int64, error) {
	var count int64
	err := r.queryRow(ctx, `
SELECT COUNT(*)
FROM act_participation_records
WHERE campaign_id = $1 AND prize_id = $2
`, campaignID, prizeID).Scan(&count)
	return count, err
}

func (r *activityCenterRepository) ListPrizeIssuedCodes(ctx context.Context, campaignID int64, prizeID string) ([]string, error) {
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT reward_payload_json ->> 'code'
FROM act_participation_records
WHERE campaign_id = $1
  AND prize_id = $2
  AND reward_payload_json ? 'code'
ORDER BY id ASC
`, campaignID, prizeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codes := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		if code != "" {
			codes = append(codes, code)
		}
	}
	return codes, rows.Err()
}

func (r *activityCenterRepository) UserHasAllowedGroup(ctx context.Context, userID int64, groupIDs []int64) (bool, error) {
	if len(groupIDs) == 0 {
		return true, nil
	}
	args := make([]any, 0, len(groupIDs)+1)
	args = append(args, userID)
	placeholders := make([]string, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	if len(placeholders) == 0 {
		return true, nil
	}
	var exists bool
	err := r.queryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM user_allowed_groups
	WHERE user_id = $1
	  AND group_id IN (`+strings.Join(placeholders, ",")+`)
)
`, args...).Scan(&exists)
	return exists, err
}

func (r *activityCenterRepository) ListUserAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT DISTINCT group_id
FROM user_allowed_groups
WHERE user_id = $1
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groupIDs := make([]int64, 0)
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (r *activityCenterRepository) ListRecords(
	ctx context.Context,
	params pagination.PaginationParams,
	filters activitycenter.RecordFilters,
) ([]activitycenter.Record, *pagination.PaginationResult, error) {
	where, args := activityRecordWhere(filters)
	totalQuery := "SELECT COUNT(*) FROM act_participation_records r LEFT JOIN users u ON u.id = r.user_id " + where

	var total int64
	if err := r.queryRow(ctx, totalQuery, args...).Scan(&total); err != nil {
		return nil, nil, err
	}

	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderDesc))
	orderBy := " ORDER BY r.created_at " + sortOrder + ", r.id " + sortOrder
	args = append(args, params.Limit(), params.Offset())
	rows, err := r.executor(ctx).QueryContext(ctx, `
SELECT r.id, r.campaign_id, r.campaign_title, r.campaign_type, r.user_id,
       COALESCE(u.email, ''), COALESCE(u.username, ''),
       r.pool_id, r.pool_name, r.prize_id, r.prize_label, r.prize_type, r.prize_color,
       r.result_status, r.reward_status, r.reward_payload_json, r.created_at
FROM act_participation_records r
LEFT JOIN users u ON u.id = r.user_id
`+where+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args))+`
`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]activitycenter.Record, 0)
	for rows.Next() {
		var item activitycenter.Record
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.CampaignTitle,
			&item.CampaignType,
			&item.UserID,
			&item.UserEmail,
			&item.UserName,
			&item.PoolID,
			&item.PoolName,
			&item.PrizeID,
			&item.PrizeLabel,
			&item.PrizeType,
			&item.PrizeColor,
			&item.ResultStatus,
			&item.RewardStatus,
			&item.RewardPayloadJSON,
			&item.CreatedAt,
		); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *activityCenterRepository) getOne(ctx context.Context, where string, args ...any) (*activitycenter.Campaign, error) {
	row := r.queryRow(ctx, `
SELECT id, title, subtitle, banner_url, banner_html, type, ref_id, config_json, status, starts_at, ends_at,
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

func activityRecordWhere(filters activitycenter.RecordFilters) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if filters.UserID > 0 {
		args = append(args, filters.UserID)
		clauses = append(clauses, fmt.Sprintf("r.user_id = $%d", len(args)))
	}
	if filters.CampaignID > 0 {
		args = append(args, filters.CampaignID)
		clauses = append(clauses, fmt.Sprintf("r.campaign_id = $%d", len(args)))
	}
	if filters.Type != "" {
		args = append(args, filters.Type)
		clauses = append(clauses, fmt.Sprintf("r.campaign_type = $%d", len(args)))
	}
	if filters.Search != "" {
		args = append(args, "%"+filters.Search+"%")
		clauses = append(clauses, fmt.Sprintf("(r.campaign_title ILIKE $%d OR r.prize_label ILIKE $%d OR u.email ILIKE $%d OR u.username ILIKE $%d)", len(args), len(args), len(args), len(args)))
	}
	if len(clauses) == 0 {
		return "", args
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

type activityCenterQueryRow struct {
	rows *sql.Rows
	err  error
}

func (r *activityCenterQueryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.rows == nil {
		return sql.ErrNoRows
	}
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := r.rows.Scan(dest...); err != nil {
		return err
	}
	return r.rows.Err()
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
		&item.BannerHTML,
		&item.Type,
		&item.RefID,
		&item.ConfigJSON,
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
