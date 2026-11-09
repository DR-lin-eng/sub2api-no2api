package service

import "context"

type imageIndependentSchedulingKey struct{}

type imageSchedulingPolicy struct {
	enabled      bool
	imageRequest bool
}

// WithImageIndependentScheduling marks a trusted image scheduling request.
// Only the persistent schedulable switch is bypassed; account health still applies.
func WithImageIndependentScheduling(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, imageIndependentSchedulingKey{}, imageSchedulingPolicy{enabled: true, imageRequest: true})
}

func ImageIndependentSchedulingFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	policy, _ := ctx.Value(imageIndependentSchedulingKey{}).(imageSchedulingPolicy)
	return policy.enabled && policy.imageRequest
}

func imageSchedulingContext(ctx context.Context, settings *SettingService, model string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	imageRequest := OpenAIImagesEndpointFromContext(ctx) || isOpenAIImageGenerationModel(model) || isGeminiImageOutputModel(normalizeAntigravityModelName(model))
	if policy, prepared := ctx.Value(imageIndependentSchedulingKey{}).(imageSchedulingPolicy); prepared {
		if policy.imageRequest == imageRequest {
			return ctx
		}
		policy.imageRequest = imageRequest
		return context.WithValue(ctx, imageIndependentSchedulingKey{}, policy)
	}
	if !imageRequest {
		return ctx
	}
	return context.WithValue(ctx, imageIndependentSchedulingKey{}, imageSchedulingPolicy{
		enabled: settings.ImageIndependentSchedulingEnabled(ctx), imageRequest: true,
	})
}

// WithImageScheduling pins the gateway policy for selection and queued-slot revalidation.
func (s *GatewayService) WithImageScheduling(ctx context.Context, model string) context.Context {
	if s == nil {
		return imageSchedulingContext(ctx, nil, model)
	}
	return imageSchedulingContext(ctx, s.settingService, model)
}

func (s *OpenAIGatewayService) WithImageScheduling(ctx context.Context, model string) context.Context {
	if s == nil {
		return imageSchedulingContext(ctx, nil, model)
	}
	return imageSchedulingContext(ctx, s.settingService, model)
}
