package dto

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
)

func TestUserActivityCampaignFromService_RedactsLotteryWeights(t *testing.T) {
	t.Parallel()

	campaign := &activitycenter.Campaign{
		Type:       activitycenter.CampaignTypeLottery,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","tier":"basic","required_group_ids":[11,12],"sort_order":9,"daily_limit":1,"prizes":[{"id":"prize-1","label":"A","prize_type":"balance","color":"#22c55e","weight":25,"available_count":10,"value_amount":"5","reward_group_id":7,"codes":["SECRET"],"is_fallback":false,"sort_order":1},{"id":"prize-2","label":"B","weight":75}]}]}}`,
	}

	out := UserActivityCampaignFromService(campaign)
	require.NotNil(t, out)
	require.NotContains(t, out.ConfigJSON, "weight")
	require.NotContains(t, out.ConfigJSON, "available_count")
	require.NotContains(t, out.ConfigJSON, "value_amount")
	require.NotContains(t, out.ConfigJSON, "reward_group_id")
	require.NotContains(t, out.ConfigJSON, "codes")
	require.NotContains(t, out.ConfigJSON, "SECRET")
	require.NotContains(t, out.ConfigJSON, "is_fallback")
	require.Contains(t, out.ConfigJSON, `"required_group_ids":[11,12]`)
	require.Contains(t, out.ConfigJSON, `"can_draw":false`)
	require.Contains(t, out.ConfigJSON, `"daily_limit":1`)
	require.NotContains(t, out.ConfigJSON, "sort_order")
	require.NotContains(t, out.ConfigJSON, "tier")
	require.Contains(t, out.ConfigJSON, `"label":"A"`)
	require.Contains(t, out.ConfigJSON, `"label":"B"`)
	require.Contains(t, out.ConfigJSON, `"prize_type":"balance"`)
	require.Contains(t, out.ConfigJSON, `"color":"#22c55e"`)
}

func TestActivityParticipationRecordFromService_RedactsPrivateRewardPayloadForUser(t *testing.T) {
	t.Parallel()

	record := &activitycenter.Record{
		ID:                1,
		CampaignID:        7,
		CampaignTitle:     "Lucky",
		CampaignType:      activitycenter.CampaignTypeLottery,
		UserID:            1001,
		PrizeID:           "prize-1",
		PrizeLabel:        "Balance",
		PrizeType:         "balance",
		PrizeColor:        "#22c55e",
		ResultStatus:      "won",
		RewardStatus:      "pending",
		RewardPayloadJSON: `{"value_amount":"5","reward_group_id":9,"code":"SECRET"}`,
	}

	userOut := ActivityParticipationRecordFromService(record, false)
	adminOut := ActivityParticipationRecordFromService(record, true)

	require.Empty(t, userOut.RewardPayloadJSON)
	require.Equal(t, "5", userOut.RewardValue)
	require.Equal(t, "SECRET", userOut.RewardCode)
	require.Contains(t, adminOut.RewardPayloadJSON, "SECRET")
}

func TestActivityParticipationRecordFromService_MapsCheckinRewardValueForAdmin(t *testing.T) {
	t.Parallel()

	record := &activitycenter.Record{
		CampaignType:      activitycenter.CampaignTypeCheckin,
		RewardStatus:      "granted",
		RewardPayloadJSON: `{"day":1,"reward_type":"balance","value":"10","label":"签到奖励"}`,
	}

	out := ActivityParticipationRecordFromService(record, true)

	require.Equal(t, "10", out.RewardValue)
	require.Equal(t, record.RewardPayloadJSON, out.RewardPayloadJSON)
}

func TestPublicCustomActivityConfigPreservesActionsWithoutPrivateFields(t *testing.T) {
	campaign := &activitycenter.Campaign{Type: activitycenter.CampaignTypeCustom, ConfigJSON: `{"custom":{"action_label":"Join","action_hint":"Once ready"},"lottery":{"pools":[{"prizes":[{"codes":["private-code"]}]}]}}`}
	raw := publicActivityCampaignConfigJSON(campaign)
	require.Contains(t, raw, `"action_label":"Join"`)
	require.NotContains(t, raw, "private-code")
}
