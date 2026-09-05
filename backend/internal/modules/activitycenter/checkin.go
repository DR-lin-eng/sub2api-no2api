package activitycenter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func CheckinCycleDays(cycleType string) int {
	switch cycleType {
	case "biweekly":
		return 14
	case "monthly":
		return 30
	default:
		return 7
	}
}

func (s *Service) CheckinLeaderboard(ctx context.Context, campaignID int64) ([]CheckinLeaderboardEntry, error) {
	items, err := s.repo.ListCheckinLeaderboard(ctx, campaignID, 10)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Rank = i + 1
		if strings.TrimSpace(items[i].UserName) != "" {
			items[i].UserName = maskCheckinName(items[i].UserName)
		} else {
			items[i].UserName = maskCheckinEmail(items[i].UserEmail)
		}
		items[i].UserEmail = ""
	}
	return items, nil
}

func maskCheckinName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*"
	}
	runes := []rune(value)
	if len(runes) == 1 {
		return "*"
	}
	if len(runes) == 2 {
		return string(runes[:1]) + "*"
	}
	return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
}

func maskCheckinEmail(value string) string {
	value = strings.TrimSpace(value)
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return "匿名用户"
	}
	local, domain := value[:at], value[at+1:]
	localRunes := []rune(local)
	if len(localRunes) == 1 {
		return "*@" + domain
	}
	return string(localRunes[:1]) + "***@" + domain
}

func ParseCheckinConfig(campaign *Campaign) (*CheckinConfig, error) {
	if campaign == nil || campaign.Type != CampaignTypeCheckin {
		return nil, ErrCampaignTypeInvalid
	}
	config, err := parseActivityConfig(campaign.ConfigJSON)
	if err != nil || config.Checkin == nil || validateCheckinConfig(config) != nil {
		return nil, ErrCampaignConfigInvalid
	}
	return config.Checkin, nil
}

func checkinLocation(config *CheckinConfig) (*time.Location, error) {
	zone := strings.TrimSpace(config.Timezone)
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	return time.LoadLocation(zone)
}

func (s *Service) CheckinStatus(ctx context.Context, campaignID, userID int64) (*CheckinStatus, error) {
	campaign, err := s.repo.GetVisibleByID(ctx, campaignID, time.Now())
	if err != nil {
		return nil, err
	}
	config, err := ParseCheckinConfig(campaign)
	if err != nil {
		return nil, err
	}
	location, err := checkinLocation(config)
	if err != nil {
		return nil, ErrCampaignConfigInvalid
	}
	now := time.Now().In(location)
	return s.checkinStatus(ctx, campaignID, userID, now, CheckinCycleDays(config.CycleType))
}

func (s *Service) checkinStatus(ctx context.Context, campaignID, userID int64, now time.Time, cycleDays int) (*CheckinStatus, error) {
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return s.repo.GetCheckinStatus(ctx, campaignID, userID, date, cycleDays)
}

func (s *Service) Checkin(ctx context.Context, campaignID, userID int64) (*Record, *CheckinStatus, error) {
	if userID <= 0 {
		return nil, nil, ErrCampaignInputRequired
	}
	var out *Record
	var status *CheckinStatus
	var grant *RewardGrant
	err := s.repo.WithCheckinTx(ctx, campaignID, userID, func(txCtx context.Context) error {
		campaign, err := s.repo.GetVisibleByID(txCtx, campaignID, time.Now())
		if err != nil {
			return err
		}
		config, err := ParseCheckinConfig(campaign)
		if err != nil {
			return err
		}
		if ok, err := s.repo.UserHasAllowedGroup(txCtx, userID, config.RequiredGroupIDs); err != nil {
			return err
		} else if !ok {
			return ErrCampaignNotEligible
		}
		location, err := checkinLocation(config)
		if err != nil {
			return ErrCampaignConfigInvalid
		}
		now := time.Now().In(location)
		status, err = s.checkinStatus(txCtx, campaignID, userID, now, CheckinCycleDays(config.CycleType))
		if err != nil {
			return err
		}
		if status.CheckedToday {
			return ErrCampaignAlreadyCheckedIn
		}
		streakDays := 1
		if status.LastCheckinDate != nil && status.LastCheckinDate.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02") {
			streakDays = status.StreakDays + 1
		}
		cycleDays := CheckinCycleDays(config.CycleType)
		cycleDay := (streakDays-1)%cycleDays + 1
		cycleNo := (streakDays-1)/cycleDays + 1
		var reward *CheckinReward
		for i := range config.DailyRewards {
			if config.DailyRewards[i].Day == cycleDay {
				reward = &config.DailyRewards[i]
				break
			}
		}
		if reward == nil {
			return ErrCampaignConfigInvalid
		}
		g := RewardGrant{PrizeType: reward.RewardType, ValueAmount: reward.Value, GroupID: reward.RewardGroupID, Description: reward.Label}
		if s.rewardGranter == nil {
			return ErrCampaignRewardFailed
		}
		if err := s.rewardGranter.Grant(txCtx, userID, g); err != nil {
			return err
		}
		grant = &g
		payload, _ := json.Marshal(map[string]any{"day": cycleDay, "streak_days": streakDays, "reward_type": reward.RewardType, "value": reward.Value, "label": reward.Label})
		checkin := &CheckinRecord{CampaignID: campaignID, UserID: userID, CheckinDate: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), CycleNo: cycleNo, CycleDay: cycleDay, StreakDays: streakDays, RewardType: reward.RewardType, RewardValue: reward.Value, RewardStatus: "granted", RewardPayloadJSON: string(payload)}
		if err := s.repo.CreateCheckinRecord(txCtx, checkin); err != nil {
			return fmt.Errorf("create checkin record: %w", err)
		}
		out = &Record{CampaignID: campaignID, CampaignTitle: campaign.Title, CampaignType: CampaignTypeCheckin, UserID: userID, PrizeLabel: reward.Label, PrizeType: reward.RewardType, ResultStatus: "won", RewardStatus: "granted", RewardPayloadJSON: string(payload)}
		if err := s.repo.CreateRecord(txCtx, out); err != nil {
			return err
		}
		status.CheckedToday = true
		status.StreakDays = streakDays
		status.CycleDay = cycleDay
		status.LastCheckinDate = &checkin.CheckinDate
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if grant != nil {
		s.repo.AfterCommit(ctx, func() { s.rewardGranter.AfterCommit(ctx, userID, *grant) })
	}
	return out, status, nil
}
