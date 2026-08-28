package activitycenter

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

const (
	CampaignTypeLottery      = "lottery"
	CampaignTypeRedeem       = "redeem"
	CampaignTypeExternalLink = "external_link"
	CampaignTypeCustom       = "custom"
)

const (
	CampaignStatusDraft    = "draft"
	CampaignStatusActive   = "active"
	CampaignStatusArchived = "archived"
)

var (
	ErrCampaignNotFound        = infraerrors.NotFound("ACTIVITY_CAMPAIGN_NOT_FOUND", "activity campaign not found")
	ErrCampaignInputRequired   = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_INPUT_REQUIRED", "activity campaign input is required")
	ErrCampaignTitleInvalid    = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TITLE_INVALID", "activity campaign title is invalid")
	ErrCampaignSubtitleInvalid = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_SUBTITLE_INVALID", "activity campaign subtitle is invalid")
	ErrCampaignBannerInvalid   = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_BANNER_INVALID", "activity campaign banner_url is invalid")
	ErrCampaignRefInvalid      = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_REF_INVALID", "activity campaign ref_id is invalid")
	ErrCampaignTypeInvalid     = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TYPE_INVALID", "activity campaign type is invalid")
	ErrCampaignStatusInvalid   = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_STATUS_INVALID", "activity campaign status is invalid")
	ErrCampaignContentInvalid  = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_CONTENT_INVALID", "activity campaign content is invalid")
	ErrCampaignSortInvalid     = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_SORT_INVALID", "activity campaign sort_order is invalid")
	ErrCampaignScheduleInvalid = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TIME_RANGE_INVALID", "starts_at must be before ends_at")
)

type Campaign struct {
	ID         int64
	Title      string
	Subtitle   string
	BannerURL  string
	BannerHTML string
	Type       string
	RefID      string
	Status     string
	StartsAt   *time.Time
	EndsAt     *time.Time
	SortOrder  int
	Content    string
	CreatedBy  *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (c *Campaign) IsVisibleAt(now time.Time) bool {
	if c == nil {
		return false
	}
	if c.Status != CampaignStatusActive {
		return false
	}
	if c.StartsAt != nil && now.Before(*c.StartsAt) {
		return false
	}
	if c.EndsAt != nil && !now.Before(*c.EndsAt) {
		return false
	}
	return true
}

type ListFilters struct {
	Status string
	Type   string
	Search string
}

type Repository interface {
	Create(ctx context.Context, campaign *Campaign) error
	GetByID(ctx context.Context, id int64) (*Campaign, error)
	GetVisibleByID(ctx context.Context, id int64, now time.Time) (*Campaign, error)
	Update(ctx context.Context, campaign *Campaign) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, filters ListFilters) ([]Campaign, *pagination.PaginationResult, error)
	ListVisible(ctx context.Context, now time.Time) ([]Campaign, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	Title      string
	Subtitle   string
	BannerURL  string
	BannerHTML string
	Type       string
	RefID      string
	Status     string
	StartsAt   *time.Time
	EndsAt     *time.Time
	SortOrder  int
	Content    string
	ActorID    *int64
}

type UpdateInput struct {
	Title      *string
	Subtitle   *string
	BannerURL  *string
	BannerHTML *string
	Type       *string
	RefID      *string
	Status     *string
	StartsAt   **time.Time
	EndsAt     **time.Time
	SortOrder  *int
	Content    *string
}

func (s *Service) Create(ctx context.Context, input *CreateInput) (*Campaign, error) {
	if input == nil {
		return nil, ErrCampaignInputRequired
	}

	campaign := &Campaign{
		Title:      input.Title,
		Subtitle:   input.Subtitle,
		BannerURL:  input.BannerURL,
		BannerHTML: input.BannerHTML,
		Type:       input.Type,
		RefID:      input.RefID,
		Status:     input.Status,
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
		SortOrder:  input.SortOrder,
		Content:    input.Content,
	}
	if input.ActorID != nil && *input.ActorID > 0 {
		campaign.CreatedBy = input.ActorID
	}
	if err := normalizeCampaign(campaign); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (s *Service) Update(ctx context.Context, id int64, input *UpdateInput) (*Campaign, error) {
	if input == nil {
		return nil, ErrCampaignInputRequired
	}

	campaign, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		campaign.Title = *input.Title
	}
	if input.Subtitle != nil {
		campaign.Subtitle = *input.Subtitle
	}
	if input.BannerURL != nil {
		campaign.BannerURL = *input.BannerURL
	}
	if input.BannerHTML != nil {
		campaign.BannerHTML = *input.BannerHTML
	}
	if input.Type != nil {
		campaign.Type = *input.Type
	}
	if input.RefID != nil {
		campaign.RefID = *input.RefID
	}
	if input.Status != nil {
		campaign.Status = *input.Status
	}
	if input.StartsAt != nil {
		campaign.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		campaign.EndsAt = *input.EndsAt
	}
	if input.SortOrder != nil {
		campaign.SortOrder = *input.SortOrder
	}
	if input.Content != nil {
		campaign.Content = *input.Content
	}

	if err := normalizeCampaign(campaign); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) Get(ctx context.Context, id int64) (*Campaign, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetVisible(ctx context.Context, id int64) (*Campaign, error) {
	item, err := s.repo.GetVisibleByID(ctx, id, time.Now())
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, params pagination.PaginationParams, filters ListFilters) ([]Campaign, *pagination.PaginationResult, error) {
	filters.Status = strings.TrimSpace(filters.Status)
	filters.Type = strings.TrimSpace(filters.Type)
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}
	return s.repo.List(ctx, params, filters)
}

func (s *Service) ListVisible(ctx context.Context) ([]Campaign, error) {
	return s.repo.ListVisible(ctx, time.Now())
}

func normalizeCampaign(c *Campaign) error {
	if c == nil {
		return ErrCampaignInputRequired
	}

	c.Title = strings.TrimSpace(c.Title)
	c.Subtitle = strings.TrimSpace(c.Subtitle)
	c.BannerURL = strings.TrimSpace(c.BannerURL)
	c.BannerHTML = strings.TrimSpace(c.BannerHTML)
	c.Type = strings.TrimSpace(c.Type)
	c.RefID = strings.TrimSpace(c.RefID)
	c.Status = strings.TrimSpace(c.Status)
	c.Content = strings.TrimSpace(c.Content)

	if c.Title == "" || len(c.Title) > 200 {
		return ErrCampaignTitleInvalid
	}
	if len(c.Subtitle) > 500 {
		return ErrCampaignSubtitleInvalid
	}
	if len(c.BannerURL) > 500 {
		return ErrCampaignBannerInvalid
	}
	if len(c.BannerHTML) > 20000 {
		return ErrCampaignBannerInvalid
	}
	if len(c.RefID) > 200 {
		return ErrCampaignRefInvalid
	}
	if c.Type == "" {
		c.Type = CampaignTypeCustom
	}
	if !isValidCampaignType(c.Type) {
		return ErrCampaignTypeInvalid
	}
	if c.Status == "" {
		c.Status = CampaignStatusDraft
	}
	if !isValidCampaignStatus(c.Status) {
		return ErrCampaignStatusInvalid
	}
	if len(c.Content) > 50000 {
		return ErrCampaignContentInvalid
	}
	if c.SortOrder < 0 || c.SortOrder > 10000 {
		return ErrCampaignSortInvalid
	}
	if c.StartsAt != nil && c.EndsAt != nil && !c.StartsAt.Before(*c.EndsAt) {
		return ErrCampaignScheduleInvalid
	}
	return nil
}

func isValidCampaignType(value string) bool {
	switch value {
	case CampaignTypeLottery, CampaignTypeRedeem, CampaignTypeExternalLink, CampaignTypeCustom:
		return true
	default:
		return false
	}
}

func isValidCampaignStatus(value string) bool {
	switch value {
	case CampaignStatusDraft, CampaignStatusActive, CampaignStatusArchived:
		return true
	default:
		return false
	}
}
