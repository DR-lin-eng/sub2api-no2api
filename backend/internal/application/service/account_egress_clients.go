package service

import (
	"context"
	"strings"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
)

func newAntigravityClient(ctx context.Context, proxyURL string) (*antigravity.Client, error) {
	if strings.TrimSpace(proxyURL) != "" {
		return antigravity.NewClient(proxyURL)
	}
	routed, ok := platformegress.FromContext(ctx)
	if !ok {
		return antigravity.NewClient("")
	}
	effective, err := platformegress.ApplyPolicy(routed.Route, routed.Policy)
	if err != nil {
		return nil, err
	}
	if effective.Mode != platformegress.ModeIPv6Pool {
		return antigravity.NewClient("")
	}
	dialContext, err := platformegress.NewDialContext(effective, routed.Policy, platformegress.DialerOptions{})
	if err != nil {
		return nil, err
	}
	return antigravity.NewClientWithDialContext("", dialContext)
}
