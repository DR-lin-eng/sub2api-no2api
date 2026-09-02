package dto

import (
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
)

type ActivityCampaign struct {
	ID              int64                    `json:"id"`
	Title           string                   `json:"title"`
	Subtitle        string                   `json:"subtitle"`
	BannerURL       string                   `json:"banner_url"`
	BannerHTML      string                   `json:"banner_html"`
	Type            string                   `json:"type"`
	RefID           string                   `json:"ref_id"`
	ConfigJSON      string                   `json:"config_json"`
	Status          string                   `json:"status"`
	EffectiveStatus string                   `json:"effective_status"`
	StartsAt        *time.Time               `json:"starts_at,omitempty"`
	EndsAt          *time.Time               `json:"ends_at,omitempty"`
	SortOrder       int                      `json:"sort_order"`
	Content         string                   `json:"content"`
	CreatedBy       *int64                   `json:"created_by,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	PrizeStockStats []ActivityPrizeStockStat `json:"prize_stock_stats,omitempty"`
}

type ActivityPrizeStockStat struct {
	PoolID         string `json:"pool_id"`
	PrizeID        string `json:"prize_id"`
	IssuedCount    int64  `json:"issued_count"`
	AvailableCount *int64 `json:"available_count,omitempty"`
	RemainingCount *int64 `json:"remaining_count,omitempty"`
}

type UserActivityCampaign struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Subtitle   string     `json:"subtitle"`
	BannerHTML string     `json:"banner_html"`
	Type       string     `json:"type"`
	ConfigJSON string     `json:"config_json"`
	StartsAt   *time.Time `json:"starts_at,omitempty"`
	EndsAt     *time.Time `json:"ends_at,omitempty"`
	Content    string     `json:"content"`
}

type ActivityParticipationRecord struct {
	ID                int64     `json:"id"`
	CampaignID        int64     `json:"campaign_id"`
	CampaignTitle     string    `json:"campaign_title"`
	CampaignType      string    `json:"campaign_type"`
	UserID            int64     `json:"user_id"`
	UserEmail         string    `json:"user_email"`
	UserName          string    `json:"user_name"`
	PoolID            string    `json:"pool_id"`
	PoolName          string    `json:"pool_name"`
	PrizeID           string    `json:"prize_id"`
	PrizeLabel        string    `json:"prize_label"`
	PrizeType         string    `json:"prize_type"`
	PrizeColor        string    `json:"prize_color"`
	ResultStatus      string    `json:"result_status"`
	RewardStatus      string    `json:"reward_status"`
	RewardValue       string    `json:"reward_value,omitempty"`
	InflatePct        *float64  `json:"inflate_pct,omitempty"`
	RewardCode        string    `json:"reward_code,omitempty"`
	RewardPayloadJSON string    `json:"reward_payload_json,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ActivityCheckinStatus struct {
	CheckedToday    bool       `json:"checked_today"`
	StreakDays      int        `json:"streak_days"`
	CycleDay        int        `json:"cycle_day"`
	LastCheckinDate *time.Time `json:"last_checkin_date,omitempty"`
}

type ActivityCheckinLeaderboardEntry struct {
	Rank         int    `json:"rank"`
	UserName     string `json:"username"`
	StreakDays   int    `json:"streak_days"`
	CheckinCount int    `json:"checkin_count"`
}

func ActivityCheckinLeaderboardFromService(items []activitycenter.CheckinLeaderboardEntry) []ActivityCheckinLeaderboardEntry {
	out := make([]ActivityCheckinLeaderboardEntry, 0, len(items))
	for _, item := range items {
		out = append(out, ActivityCheckinLeaderboardEntry{Rank: item.Rank, UserName: item.UserName, StreakDays: item.StreakDays, CheckinCount: item.CheckinCount})
	}
	return out
}

func ActivityCheckinStatusFromService(status *activitycenter.CheckinStatus) *ActivityCheckinStatus {
	if status == nil {
		return nil
	}
	return &ActivityCheckinStatus{CheckedToday: status.CheckedToday, StreakDays: status.StreakDays, CycleDay: status.CycleDay, LastCheckinDate: status.LastCheckinDate}
}

func ActivityCampaignFromService(campaign *activitycenter.Campaign) *ActivityCampaign {
	if campaign == nil {
		return nil
	}
	return &ActivityCampaign{
		ID:              campaign.ID,
		Title:           campaign.Title,
		Subtitle:        campaign.Subtitle,
		BannerURL:       campaign.BannerURL,
		BannerHTML:      campaign.BannerHTML,
		Type:            campaign.Type,
		RefID:           campaign.RefID,
		ConfigJSON:      campaign.ConfigJSON,
		Status:          campaign.Status,
		EffectiveStatus: effectiveCampaignStatus(campaign, time.Now()),
		StartsAt:        campaign.StartsAt,
		EndsAt:          campaign.EndsAt,
		SortOrder:       campaign.SortOrder,
		Content:         campaign.Content,
		CreatedBy:       campaign.CreatedBy,
		CreatedAt:       campaign.CreatedAt,
		UpdatedAt:       campaign.UpdatedAt,
	}
}

func effectiveCampaignStatus(campaign *activitycenter.Campaign, now time.Time) string {
	if campaign == nil {
		return ""
	}
	if campaign.Status != activitycenter.CampaignStatusActive {
		return campaign.Status
	}
	if campaign.StartsAt != nil && now.Before(*campaign.StartsAt) {
		return "scheduled"
	}
	if campaign.EndsAt != nil && !now.Before(*campaign.EndsAt) {
		return "ended"
	}
	return activitycenter.CampaignStatusActive
}

func ActivityPrizeStockStatsFromService(stats []activitycenter.PrizeStockStat) []ActivityPrizeStockStat {
	if len(stats) == 0 {
		return nil
	}
	out := make([]ActivityPrizeStockStat, 0, len(stats))
	for _, stat := range stats {
		out = append(out, ActivityPrizeStockStat{
			PoolID:         stat.PoolID,
			PrizeID:        stat.PrizeID,
			IssuedCount:    stat.IssuedCount,
			AvailableCount: stat.AvailableCount,
			RemainingCount: stat.RemainingCount,
		})
	}
	return out
}

func UserActivityCampaignFromService(campaign *activitycenter.Campaign) *UserActivityCampaign {
	if campaign == nil {
		return nil
	}
	return &UserActivityCampaign{
		ID:         campaign.ID,
		Title:      campaign.Title,
		Subtitle:   campaign.Subtitle,
		BannerHTML: campaign.BannerHTML,
		Type:       campaign.Type,
		ConfigJSON: publicActivityCampaignConfigJSON(campaign),
		StartsAt:   campaign.StartsAt,
		EndsAt:     campaign.EndsAt,
		Content:    campaign.Content,
	}
}

func ActivityParticipationRecordFromService(record *activitycenter.Record, includePrivate bool) *ActivityParticipationRecord {
	if record == nil {
		return nil
	}
	out := &ActivityParticipationRecord{
		ID:            record.ID,
		CampaignID:    record.CampaignID,
		CampaignTitle: record.CampaignTitle,
		CampaignType:  record.CampaignType,
		UserID:        record.UserID,
		UserEmail:     record.UserEmail,
		UserName:      record.UserName,
		PoolID:        record.PoolID,
		PoolName:      record.PoolName,
		PrizeID:       record.PrizeID,
		PrizeLabel:    record.PrizeLabel,
		PrizeType:     record.PrizeType,
		PrizeColor:    record.PrizeColor,
		ResultStatus:  record.ResultStatus,
		RewardStatus:  record.RewardStatus,
		CreatedAt:     record.CreatedAt,
	}
	var reward struct {
		ValueAmount string   `json:"value_amount"`
		Value       string   `json:"value"`
		Code        string   `json:"code"`
		InflatePct  *float64 `json:"inflate_pct"`
	}
	if json.Unmarshal([]byte(record.RewardPayloadJSON), &reward) == nil {
		out.RewardValue = reward.ValueAmount
		if reward.Value != "" {
			out.RewardValue = reward.Value
		}
		out.RewardCode = reward.Code
		out.InflatePct = reward.InflatePct
	}
	if includePrivate {
		out.RewardPayloadJSON = record.RewardPayloadJSON
	} else {
		out.RewardPayloadJSON = ""
	}
	return out
}

func publicActivityCampaignConfigJSON(campaign *activitycenter.Campaign) string {
	if campaign == nil {
		return "{}"
	}
	if campaign.Type == activitycenter.CampaignTypeInflate || campaign.Type == activitycenter.CampaignTypeRedeem {
		var config activitycenter.ActivityConfig
		if err := json.Unmarshal([]byte(campaign.ConfigJSON), &config); err != nil {
			return "{}"
		}
		rule := config.Inflate
		if rule == nil {
			rule = config.Redeem
		}
		if rule == nil {
			return "{}"
		}
		payload := map[string]any{"inflate": rule}
		out, err := json.Marshal(payload)
		if err != nil {
			return "{}"
		}
		return string(out)
	}
	if campaign.Type != activitycenter.CampaignTypeLottery {
		if campaign.Type == activitycenter.CampaignTypeCheckin {
			var config activitycenter.ActivityConfig
			if err := json.Unmarshal([]byte(campaign.ConfigJSON), &config); err != nil || config.Checkin == nil {
				return "{}"
			}
			out, err := json.Marshal(map[string]any{"checkin": config.Checkin})
			if err != nil {
				return "{}"
			}
			return string(out)
		}
		return "{}"
	}
	return sanitizePublicActivityCampaignConfigJSON(campaign.ConfigJSON)
}

func sanitizePublicActivityCampaignConfigJSON(raw string) string {
	if raw == "" {
		return "{}"
	}

	var config activitycenter.ActivityConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil || config.Lottery == nil {
		return "{}"
	}
	pools := make([]map[string]any, 0, len(config.Lottery.Pools))
	for _, pool := range config.Lottery.Pools {
		if pool.Enabled != nil && !*pool.Enabled {
			continue
		}
		prizes := make([]map[string]any, 0, len(pool.Prizes))
		for _, prize := range pool.Prizes {
			if prize.Weight <= 0 {
				continue
			}
			prizes = append(prizes, map[string]any{
				"id":         prize.ID,
				"label":      prize.Label,
				"prize_type": prize.PrizeType,
				"color":      prize.Color,
			})
		}
		pools = append(pools, map[string]any{
			"id":                 pool.ID,
			"name":               pool.Name,
			"description":        pool.Description,
			"required_group_ids": pool.RequiredGroupIDs,
			"can_draw":           pool.CanDraw,
			"daily_limit":        pool.DailyLimit,
			"prizes":             prizes,
		})
	}
	sanitized, err := json.Marshal(map[string]any{"lottery": map[string]any{"pools": pools}})
	if err != nil {
		return "{}"
	}
	return string(sanitized)
}
