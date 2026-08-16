package service

// accountQuotaCost returns the same account-side amount shown by account usage
// statistics: an explicit account pricing result wins over the default raw
// user cost, and the account multiplier is applied last.
func (p *postUsageBillingParams) accountQuotaCost() float64 {
	if p == nil || p.Cost == nil {
		return 0
	}
	baseCost := p.Cost.TotalCost
	if p.AccountStatsCost != nil {
		baseCost = *p.AccountStatsCost
	}
	return baseCost * p.AccountRateMultiplier
}
