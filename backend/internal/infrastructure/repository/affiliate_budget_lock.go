package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func (r *affiliateRepository) WithRebateBudget(ctx context.Context, inviterID int64, fn func(context.Context) error) error {
	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		rows, err := client.QueryContext(txCtx, "SELECT user_id FROM user_affiliates WHERE user_id=$1 FOR UPDATE", inviterID)
		if err != nil {
			return fmt.Errorf("lock rebate budget: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		return fn(txCtx)
	})
}
