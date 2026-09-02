package service

import "context"

type accountSchedulingReasonWriter interface {
	SetSchedulableWithReason(ctx context.Context, id int64, schedulable bool, reason string) error
}

func setAccountSchedulableWithReason(
	ctx context.Context,
	repo AccountRepository,
	id int64,
	schedulable bool,
	reason string,
) error {
	if writer, ok := repo.(accountSchedulingReasonWriter); ok {
		return writer.SetSchedulableWithReason(ctx, id, schedulable, reason)
	}

	// Compatibility path for test doubles and alternate repositories. The
	// production PostgreSQL repository implements the atomic method above.
	if schedulable {
		if err := repo.SetSchedulable(ctx, id, true); err != nil {
			return err
		}
		return repo.UpdateExtra(ctx, id, map[string]any{AccountSchedulingDisabledReasonExtraKey: nil})
	}
	if err := repo.UpdateExtra(ctx, id, map[string]any{AccountSchedulingDisabledReasonExtraKey: reason}); err != nil {
		return err
	}
	if err := repo.SetSchedulable(ctx, id, false); err != nil {
		_ = repo.UpdateExtra(ctx, id, map[string]any{AccountSchedulingDisabledReasonExtraKey: nil})
		return err
	}
	return nil
}
