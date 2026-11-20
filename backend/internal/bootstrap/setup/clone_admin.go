package setup

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
)

// CreateCloneAdministrator applies the same local credential rules used during
// initial installation. The database receipt restricts this to a new clone.
func CreateCloneAdministrator(ctx context.Context, db *sql.DB, email, password string) error {
	email = strings.TrimSpace(email)
	if !validateEmail(email) {
		return errors.New("enter a valid administrator email")
	}
	if e := validatePassword(password); e != nil {
		return e
	}
	var admin service.User
	if e := admin.SetPassword(password); e != nil {
		return e
	}
	return sitemigration.InitializeCloneAdmin(ctx, db, email, admin.PasswordHash, setupDefaultAdminConcurrency())
}
