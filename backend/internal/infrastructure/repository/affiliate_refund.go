package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/shopspring/decimal"
)

// Lock order shared by accrual, transfers, thaw and refund reversal: affiliate
// account first, then its ledger rows. Negative net quota records debt without
// removing money from the inviter's ordinary wallet.
func lockAffiliateAccount(ctx context.Context, client *dbent.Client, userID int64) (bool, error) {
	rows, err := client.QueryContext(ctx, "SELECT user_id FROM user_affiliates WHERE user_id=$1 FOR UPDATE", userID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		return false, err
	}
	return found, rows.Close()
}

func (r *affiliateRepository) ReverseOrderQuota(ctx context.Context, orderID int64, refundAmount, orderAmount float64) (float64, error) {
	if orderID <= 0 || refundAmount <= 0 || orderAmount <= 0 || math.IsNaN(refundAmount) || math.IsNaN(orderAmount) || math.IsInf(refundAmount, 0) || math.IsInf(orderAmount, 0) {
		return 0, fmt.Errorf("invalid affiliate refund proportion")
	}
	ratio := decimal.Min(decimal.NewFromFloat(refundAmount).Div(decimal.NewFromFloat(orderAmount)), decimal.NewFromInt(1))
	total := decimal.Zero
	err := r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		rows, err := client.QueryContext(txCtx, "SELECT DISTINCT user_id FROM user_affiliate_ledger WHERE source_order_id=$1 AND action='accrue' ORDER BY user_id", orderID)
		if err != nil {
			return fmt.Errorf("find source rebate owners: %w", err)
		}
		var owners []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			owners = append(owners, id)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, owner := range owners {
			if _, err := lockAffiliateAccount(txCtx, client, owner); err != nil {
				return err
			}
			awards, err := loadRefundableAffiliateAwards(txCtx, client, orderID, owner)
			if err != nil {
				return err
			}
			for _, award := range awards {
				amount, err := decimal.NewFromString(award.amount)
				if err != nil {
					return err
				}
				reversed, err := decimal.NewFromString(award.reversed)
				if err != nil {
					return err
				}
				target := amount.Mul(ratio).Round(8)
				delta := target.Sub(reversed)
				if !delta.IsPositive() {
					continue
				}
				if _, err := client.ExecContext(txCtx, "UPDATE user_affiliate_ledger SET reversed_amount=$1, updated_at=NOW() WHERE id=$2", target.String(), award.id); err != nil {
					return err
				}
				column := "aff_quota"
				if award.frozen.Valid {
					column = "aff_frozen_quota"
				}
				query := "UPDATE user_affiliates SET " + column + "=" + column + "-$1, updated_at=NOW() WHERE user_id=$2"
				if _, err := client.ExecContext(txCtx, query, delta.String(), owner); err != nil {
					return err
				}
				if _, err := client.ExecContext(txCtx, `INSERT INTO user_affiliate_ledger
 (user_id,action,amount,source_user_id,source_order_id,created_at,updated_at)
 VALUES ($1,'refund',-($2::numeric),$3,$4,NOW(),NOW())`, owner, delta.String(), award.source, orderID); err != nil {
					return err
				}
				total = total.Add(delta)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total.InexactFloat64(), nil
}

type refundableAffiliateAward struct {
	id               int64
	amount, reversed string
	source           sql.NullInt64
	frozen           sql.NullTime
}

func loadRefundableAffiliateAwards(ctx context.Context, client *dbent.Client, orderID, owner int64) ([]refundableAffiliateAward, error) {
	rows, err := client.QueryContext(ctx, `SELECT id, amount::text, reversed_amount::text, source_user_id, frozen_until
 FROM user_affiliate_ledger WHERE source_order_id=$1 AND user_id=$2 AND action='accrue' ORDER BY id FOR UPDATE`, orderID, owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var awards []refundableAffiliateAward
	for rows.Next() {
		var award refundableAffiliateAward
		if err := rows.Scan(&award.id, &award.amount, &award.reversed, &award.source, &award.frozen); err != nil {
			return nil, err
		}
		awards = append(awards, award)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return awards, rows.Close()
}
