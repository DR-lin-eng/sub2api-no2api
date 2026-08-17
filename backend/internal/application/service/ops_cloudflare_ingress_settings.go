package service

import "context"

func (s *OpsService) GetCloudflareIngressSettings(ctx context.Context) (*CloudflareIngressSettingsView, error) {
	if s == nil || s.cloudflareIngressSettings == nil {
		return nil, ErrCloudflareIngressUnavailable
	}
	return s.cloudflareIngressSettings.Get(ctx)
}

func (s *OpsService) UpdateCloudflareIngressSettings(
	ctx context.Context,
	input UpdateCloudflareIngressSettingsInput,
) (*CloudflareIngressSettingsView, error) {
	if s == nil || s.cloudflareIngressSettings == nil {
		return nil, ErrCloudflareIngressUnavailable
	}
	return s.cloudflareIngressSettings.Update(ctx, input)
}
