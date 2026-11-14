package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// Selection is advisory until the quotation has a committed order. Serialize
// reservations per merchant across users/nodes and validate its current limits.
func (s *PaymentService) reserveProviderBudget(ctx context.Context, tx *dbent.Tx, selection *payment.InstanceSelection, paymentType string, amount float64) error {
	if selection == nil {
		return nil
	} // direct legacy fixture/import construction
	id, err := strconv.ParseInt(selection.InstanceID, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid checkout provider instance")
	}
	query := tx.PaymentProviderInstance.Query().Where(paymentproviderinstance.IDEQ(id))
	if s.entClient.Driver().Dialect() == "postgres" {
		query = query.ForUpdate()
	}
	instance, err := query.Only(ctx)
	if err != nil {
		return fmt.Errorf("lock checkout provider budget: %w", err)
	}
	if !instance.Enabled || instance.ProviderKey != selection.ProviderKey {
		return infraerrors.ServiceUnavailable("NO_AVAILABLE_INSTANCE", "checkout provider is no longer available")
	}
	limits, err := payment.GetInstanceChannelLimits(instance, paymentType)
	if err != nil {
		return fmt.Errorf("read checkout provider limits: %w", err)
	}
	usage, err := payment.ProviderDailyUsage(ctx, tx.Client(), []string{selection.InstanceID}, time.Now())
	if err != nil {
		return err
	}
	if err := payment.CheckChannelBudget(limits, usage[selection.InstanceID], amount); err != nil {
		return infraerrors.TooManyRequests("PAYMENT_PROVIDER_LIMIT_EXCEEDED", err.Error())
	}
	return nil
}
