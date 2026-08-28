package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
)

type ActivityCampaign struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Subtitle   string     `json:"subtitle"`
	BannerURL  string     `json:"banner_url"`
	BannerHTML string     `json:"banner_html"`
	Type       string     `json:"type"`
	RefID      string     `json:"ref_id"`
	Status     string     `json:"status"`
	StartsAt   *time.Time `json:"starts_at,omitempty"`
	EndsAt     *time.Time `json:"ends_at,omitempty"`
	SortOrder  int        `json:"sort_order"`
	Content    string     `json:"content"`
	CreatedBy  *int64     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type UserActivityCampaign struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Subtitle   string     `json:"subtitle"`
	BannerURL  string     `json:"banner_url"`
	BannerHTML string     `json:"banner_html"`
	Type       string     `json:"type"`
	RefID      string     `json:"ref_id"`
	StartsAt   *time.Time `json:"starts_at,omitempty"`
	EndsAt     *time.Time `json:"ends_at,omitempty"`
	Content    string     `json:"content"`
}

func ActivityCampaignFromService(campaign *activitycenter.Campaign) *ActivityCampaign {
	if campaign == nil {
		return nil
	}
	return &ActivityCampaign{
		ID:         campaign.ID,
		Title:      campaign.Title,
		Subtitle:   campaign.Subtitle,
		BannerURL:  campaign.BannerURL,
		BannerHTML: campaign.BannerHTML,
		Type:       campaign.Type,
		RefID:      campaign.RefID,
		Status:     campaign.Status,
		StartsAt:   campaign.StartsAt,
		EndsAt:     campaign.EndsAt,
		SortOrder:  campaign.SortOrder,
		Content:    campaign.Content,
		CreatedBy:  campaign.CreatedBy,
		CreatedAt:  campaign.CreatedAt,
		UpdatedAt:  campaign.UpdatedAt,
	}
}

func UserActivityCampaignFromService(campaign *activitycenter.Campaign) *UserActivityCampaign {
	if campaign == nil {
		return nil
	}
	return &UserActivityCampaign{
		ID:         campaign.ID,
		Title:      campaign.Title,
		Subtitle:   campaign.Subtitle,
		BannerURL:  campaign.BannerURL,
		BannerHTML: campaign.BannerHTML,
		Type:       campaign.Type,
		RefID:      campaign.RefID,
		StartsAt:   campaign.StartsAt,
		EndsAt:     campaign.EndsAt,
		Content:    campaign.Content,
	}
}
