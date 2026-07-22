package usagestats

// AccountStats 账号使用统计
//
// cost: 账号口径费用（使用 total_cost * account_rate_multiplier）
// standard_cost: 标准费用（使用 total_cost，不含倍率）
// user_cost: 用户/API Key 口径费用（使用 actual_cost，受分组倍率影响）
type AccountStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

// AccountHourlyUsageStats summarizes one account over a recent rolling window.
// SuccessRate is a ratio in [0, 1], matching the Ops dashboard SLA convention.
type AccountHourlyUsageStats struct {
	TotalRequests      int64    `json:"total_requests"`
	SuccessfulRequests int64    `json:"successful_requests"`
	SuccessRate        float64  `json:"success_rate"`
	AvgFirstTokenMs    *float64 `json:"avg_first_token_ms"`
	Error4xx           int64    `json:"error_4xx"`
	Error5xx           int64    `json:"error_5xx"`
}
