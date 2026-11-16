package service

const errorCodeQuotaRefreshFailed = "quota_refresh_failed"

// Metadata alone cannot turn a failed probe into a fresh quota observation.
func hasOpenAIQuotaObservation(updates map[string]any) bool {
	_, fiveHour := updates["codex_5h_used_percent"]
	_, sevenDay := updates["codex_7d_used_percent"]
	return fiveHour || sevenDay
}
