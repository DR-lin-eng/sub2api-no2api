package service

type AccountQualityReasoningTokenBucket struct {
	Key   string `json:"key"`
	Min   int64  `json:"min"`
	Max   *int64 `json:"max,omitempty"`
	Count int    `json:"count"`
}

type AccountQualityReasoningTokenDistribution struct {
	AverageTokens    *float64                             `json:"average_tokens,omitempty"`
	MeasuredAccounts int                                  `json:"measured_accounts"`
	UnknownAccounts  int                                  `json:"unknown_accounts"`
	Buckets          []AccountQualityReasoningTokenBucket `json:"buckets"`
}

func newReasoningTokenDistribution() AccountQualityReasoningTokenDistribution {
	max49, max99, max249, max499, max999 := int64(49), int64(99), int64(249), int64(499), int64(999)
	return AccountQualityReasoningTokenDistribution{Buckets: []AccountQualityReasoningTokenBucket{
		{Key: "0_49", Min: 0, Max: &max49}, {Key: "50_99", Min: 50, Max: &max99},
		{Key: "100_249", Min: 100, Max: &max249}, {Key: "250_499", Min: 250, Max: &max499},
		{Key: "500_999", Min: 500, Max: &max999}, {Key: "1000_plus", Min: 1000},
	}}
}

func summarizeReasoningTokenDistribution(results []AccountInspectionAccountResult) AccountQualityReasoningTokenDistribution {
	distribution := newReasoningTokenDistribution()
	var total float64
	for _, result := range results {
		if result.QualityStage1Status == "disabled" || result.QualityStage1Status == "" {
			continue
		}
		if result.QualityReasoningTokens == nil || *result.QualityReasoningTokens < 0 {
			distribution.UnknownAccounts++
			continue
		}
		tokens := *result.QualityReasoningTokens
		distribution.MeasuredAccounts++
		total += float64(tokens)
		switch {
		case tokens < 50:
			distribution.Buckets[0].Count++
		case tokens < 100:
			distribution.Buckets[1].Count++
		case tokens < 250:
			distribution.Buckets[2].Count++
		case tokens < 500:
			distribution.Buckets[3].Count++
		case tokens < 1000:
			distribution.Buckets[4].Count++
		default:
			distribution.Buckets[5].Count++
		}
	}
	if distribution.MeasuredAccounts > 0 {
		average := total / float64(distribution.MeasuredAccounts)
		distribution.AverageTokens = &average
	}
	return distribution
}
