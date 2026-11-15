package repository

import (
	"context"

	dbaccount "github.com/DR-lin-eng/sub2api-no2api/ent/account"
	dbpredicate "github.com/DR-lin-eng/sub2api-no2api/ent/predicate"
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
)

// Image scheduling ignores only the persistent switch. Keep the active-status
// predicate even on this branch so error and disabled accounts never enter it.
func accountSchedulingSwitchPredicate(ctx context.Context) dbpredicate.Account {
	if service.ImageIndependentSchedulingFromContext(ctx) {
		return dbaccount.StatusEQ(service.StatusActive)
	}
	return dbaccount.SchedulableEQ(true)
}
