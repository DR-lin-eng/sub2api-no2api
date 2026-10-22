//go:build !darwin

package liveattestation

import "context"

type unsupportedProvider struct{ scopeKey string }

func (unsupportedProvider) ForScope(scopeKey string) Provider {
	return unsupportedProvider{scopeKey: scopeKey}
}

func NewProvider() Provider {
	return unsupportedProvider{}
}

func (p unsupportedProvider) Check(ctx context.Context) error {
	if err := validateProviderScope(ctx, p.scopeKey); err != nil {
		return err
	}
	return ErrUnsupportedPlatform
}

func (p unsupportedProvider) Generate(ctx context.Context) (string, error) {
	if err := validateProviderScope(ctx, p.scopeKey); err != nil {
		return "", err
	}
	return "", ErrUnsupportedPlatform
}

func (p unsupportedProvider) GenerateDeviceCheckToken(ctx context.Context) (string, error) {
	if err := validateProviderScope(ctx, p.scopeKey); err != nil {
		return "", err
	}
	return "", ErrUnsupportedPlatform
}

func (p unsupportedProvider) GenerateCodexToken(ctx context.Context) (string, error) {
	if err := validateProviderScope(ctx, p.scopeKey); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return codexUnavailableToken(1), nil
}
