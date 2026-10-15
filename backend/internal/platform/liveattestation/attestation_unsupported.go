//go:build !darwin

package liveattestation

import "context"

type unsupportedProvider struct{}

func NewProvider() Provider {
	return unsupportedProvider{}
}

func (unsupportedProvider) Check(context.Context) error {
	return ErrUnsupportedPlatform
}

func (unsupportedProvider) Generate(context.Context) (string, error) {
	return "", ErrUnsupportedPlatform
}

func (unsupportedProvider) GenerateDeviceCheckToken(context.Context) (string, error) {
	return "", ErrUnsupportedPlatform
}

func (unsupportedProvider) GenerateCodexToken(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return codexUnavailableToken(1), nil
}
