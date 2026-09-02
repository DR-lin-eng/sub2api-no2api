package admin

import (
	"encoding/json"
	"testing"
)

func TestUpdateGroupRequestLimitFieldsTriState(t *testing.T) {
	t.Run("omitted means unchanged", func(t *testing.T) {
		var req UpdateGroupRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatal(err)
		}
		if req.DailyLimitUSD.ToServiceInput() != nil {
			t.Fatal("omitted daily_limit_usd must remain nil")
		}
	})

	t.Run("null means unlimited", func(t *testing.T) {
		var req UpdateGroupRequest
		if err := json.Unmarshal([]byte(`{"daily_limit_usd":null}`), &req); err != nil {
			t.Fatal(err)
		}
		limit := req.DailyLimitUSD.ToServiceInput()
		if limit == nil || *limit >= 0 {
			t.Fatalf("null daily_limit_usd = %v, want explicit negative unlimited sentinel", limit)
		}
	})

	t.Run("zero remains an explicit zero limit", func(t *testing.T) {
		var req UpdateGroupRequest
		if err := json.Unmarshal([]byte(`{"daily_limit_usd":0}`), &req); err != nil {
			t.Fatal(err)
		}
		limit := req.DailyLimitUSD.ToServiceInput()
		if limit == nil || *limit != 0 {
			t.Fatalf("zero daily_limit_usd = %v, want 0", limit)
		}
	})
}
