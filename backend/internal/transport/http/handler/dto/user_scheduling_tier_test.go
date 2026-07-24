package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func TestRequestSchedulingTierIsAdminOnly(t *testing.T) {
	user := &service.User{ID: 1, SchedulingTier: service.RequestSchedulingTierLow}

	publicJSON, err := json.Marshal(UserFromService(user))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "scheduling_tier") {
		t.Fatalf("public user DTO exposed scheduling tier: %s", publicJSON)
	}

	adminJSON, err := json.Marshal(UserFromServiceAdmin(user))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(adminJSON), `"scheduling_tier":2`) {
		t.Fatalf("admin user DTO omitted scheduling tier: %s", adminJSON)
	}
}
