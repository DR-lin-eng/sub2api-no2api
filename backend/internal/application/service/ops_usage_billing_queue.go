package service

import "context"

func (s *OpsService) GetUsageBillingQueueSnapshot(ctx context.Context) (*UsageBillingQueueSnapshot, error) {
	if s == nil || s.usageBillingQueueAdmin == nil {
		return nil, ErrUsageBillingQueueUnavailable
	}
	return s.usageBillingQueueAdmin.GetUsageBillingQueueSnapshot(ctx)
}

func (s *OpsService) ListUsageBillingQueueJobs(ctx context.Context, filter UsageBillingQueueJobFilter) (*UsageBillingQueueJobList, error) {
	if s == nil || s.usageBillingQueueAdmin == nil {
		return nil, ErrUsageBillingQueueUnavailable
	}
	return s.usageBillingQueueAdmin.ListUsageBillingQueueJobs(ctx, filter)
}

func (s *OpsService) ListUsageBillingDeadLetters(ctx context.Context, filter UsageBillingQueueJobFilter) (*UsageBillingDeadLetterList, error) {
	if s == nil || s.usageBillingQueueAdmin == nil {
		return nil, ErrUsageBillingQueueUnavailable
	}
	return s.usageBillingQueueAdmin.ListUsageBillingDeadLetters(ctx, filter)
}

func (s *OpsService) RetryUsageBillingQueueJob(ctx context.Context, input UsageBillingJobRetry) error {
	if s == nil || s.usageBillingQueueAdmin == nil {
		return ErrUsageBillingQueueUnavailable
	}
	return s.usageBillingQueueAdmin.RetryUsageBillingQueueJob(ctx, input)
}

func (s *OpsService) ReplayUsageBillingDeadLetter(ctx context.Context, input UsageBillingDeadLetterReplay) error {
	if s == nil || s.usageBillingQueueAdmin == nil {
		return ErrUsageBillingQueueUnavailable
	}
	return s.usageBillingQueueAdmin.ReplayUsageBillingDeadLetter(ctx, input)
}
