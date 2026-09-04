package service

import (
	"context"
	"time"
)

type accountSchedulingReasonWriter interface {
	SetSchedulableWithReason(ctx context.Context, id int64, schedulable bool, reason string) error
}

type accountSchedulingAutoEnableWriter interface {
	SetSchedulableWithReasonAndAutoEnable(
		ctx context.Context,
		id int64,
		schedulable bool,
		reason string,
		autoEnableSource string,
		autoEnableAt *time.Time,
	) error
}

func setAccountSchedulableWithReason(
	ctx context.Context,
	repo AccountRepository,
	id int64,
	schedulable bool,
	reason string,
) error {
	return setAccountSchedulableWithReasonAndAutoEnable(ctx, repo, id, schedulable, reason, "", nil)
}

func setAccountSchedulableWithReasonAndAutoEnable(
	ctx context.Context,
	repo AccountRepository,
	id int64,
	schedulable bool,
	reason string,
	autoEnableSource string,
	autoEnableAt *time.Time,
) error {
	if writer, ok := repo.(accountSchedulingAutoEnableWriter); ok {
		return writer.SetSchedulableWithReasonAndAutoEnable(
			ctx,
			id,
			schedulable,
			reason,
			autoEnableSource,
			autoEnableAt,
		)
	}
	if writer, ok := repo.(accountSchedulingReasonWriter); ok {
		if err := writer.SetSchedulableWithReason(ctx, id, schedulable, reason); err != nil {
			return err
		}
		updates := map[string]any{
			AccountAutoEnableSourceExtraKey: nil,
			AccountAutoEnableAtExtraKey:     nil,
		}
		if autoEnableSource != "" {
			updates[AccountAutoEnableSourceExtraKey] = autoEnableSource
		}
		if autoEnableAt != nil {
			updates[AccountAutoEnableAtExtraKey] = autoEnableAt.Unix()
		}
		return repo.UpdateExtra(ctx, id, updates)
	}

	// Compatibility path for test doubles and alternate repositories. The
	// production PostgreSQL repository implements the atomic method above.
	updates := map[string]any{
		AccountSchedulingDisabledReasonExtraKey: reason,
		AccountAutoEnableSourceExtraKey:         nil,
		AccountAutoEnableAtExtraKey:             nil,
	}
	if autoEnableSource != "" {
		updates[AccountAutoEnableSourceExtraKey] = autoEnableSource
	}
	if autoEnableAt != nil {
		updates[AccountAutoEnableAtExtraKey] = autoEnableAt.Unix()
	}
	if schedulable {
		if err := repo.SetSchedulable(ctx, id, true); err != nil {
			return err
		}
		updates[AccountSchedulingDisabledReasonExtraKey] = nil
		return repo.UpdateExtra(ctx, id, updates)
	}
	if err := repo.UpdateExtra(ctx, id, updates); err != nil {
		return err
	}
	if err := repo.SetSchedulable(ctx, id, false); err != nil {
		_ = repo.UpdateExtra(ctx, id, map[string]any{
			AccountSchedulingDisabledReasonExtraKey: nil,
			AccountAutoEnableSourceExtraKey:         nil,
			AccountAutoEnableAtExtraKey:             nil,
		})
		return err
	}
	return nil
}
