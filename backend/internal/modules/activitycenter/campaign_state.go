package activitycenter

func isValidCampaignType(value string) bool {
	switch value {
	case CampaignTypeLottery, CampaignTypeInflate, CampaignTypeCheckin, CampaignTypeRedeem, CampaignTypeCustom:
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
