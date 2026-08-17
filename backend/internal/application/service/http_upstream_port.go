package service

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

func withAccountEgressContext(ctx context.Context, account *Account, proxyURL string, cfg *config.Config) context.Context {
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	return withEgressAccountRouteContext(ctx, accountEgressRoute(account, proxyURL), accountID, cfg)
}

// WithAccountEgressContext exposes the already-loaded account route to
// transport-owned follow-up work, such as asynchronous image URL offloading.
func WithAccountEgressContext(ctx context.Context, account *Account, cfg *config.Config) context.Context {
	return withAccountEgressContext(ctx, account, "", cfg)
}

func withEgressRouteContext(ctx context.Context, route platformegress.Route, cfg *config.Config) context.Context {
	return withEgressAccountRouteContext(ctx, route, 0, cfg)
}

func withEgressAccountRouteContext(ctx context.Context, route platformegress.Route, accountID int64, cfg *config.Config) context.Context {
	if route.Mode == "" {
		return ctx
	}
	policy := platformegress.Policy{FreeBind: true}
	if cfg != nil {
		policy.IPv6Enabled = cfg.IPv6Egress.Enabled
		policy.FreeBind = cfg.IPv6Egress.FreeBind
	}
	return platformegress.WithContextAccountRoute(ctx, route, policy, accountID)
}

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
type HTTPUpstream interface {
	// Do 执行 HTTP 请求（不启用 TLS 指纹）
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// profile 参数:
	//   - nil: 不启用 TLS 指纹，行为与 Do 方法相同
	//   - non-nil: 使用指定的 Profile 进行 TLS 指纹伪装
	//
	// Profile 由调用方通过 TLSFingerprintProfileService 解析后传入，
	// 支持按账号绑定的数据库 profile 或内置默认 profile。
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}

// RoutedHTTPUpstream is implemented by the production connection layer. The
// legacy interface remains intact so focused test doubles and auxiliary direct
// callers do not need to understand account egress policy.
type RoutedHTTPUpstream interface {
	DoRoute(req *http.Request, route platformegress.Route, accountID int64, accountConcurrency int) (*http.Response, error)
	DoWithTLSRoute(req *http.Request, route platformegress.Route, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}

func accountEgressRoute(account *Account, proxyURL string) platformegress.Route {
	if proxyURL != "" {
		return platformegress.ExternalProxyRoute(proxyURL)
	}
	if account == nil {
		return platformegress.DirectRoute(false)
	}
	return account.EgressRoute()
}

func doAccountHTTPUpstream(upstream HTTPUpstream, req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	concurrency := 0
	if account != nil {
		concurrency = account.Concurrency
	}
	return doAccountHTTPUpstreamWithConcurrency(upstream, req, proxyURL, account, concurrency)
}

func doAccountHTTPUpstreamWithConcurrency(upstream HTTPUpstream, req *http.Request, proxyURL string, account *Account, accountConcurrency int) (*http.Response, error) {
	route := accountEgressRoute(account, proxyURL)
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	if routed, ok := upstream.(RoutedHTTPUpstream); ok {
		return routed.DoRoute(req, route, accountID, accountConcurrency)
	}
	legacyProxyURL, err := legacyHTTPUpstreamProxyURL(route)
	if err != nil {
		return nil, err
	}
	return upstream.Do(req, legacyProxyURL, accountID, accountConcurrency)
}

func doAccountHTTPUpstreamWithTLS(upstream HTTPUpstream, req *http.Request, proxyURL string, account *Account, profile *tlsfingerprint.Profile) (*http.Response, error) {
	route := accountEgressRoute(account, proxyURL)
	accountID, accountConcurrency := int64(0), 0
	if account != nil {
		accountID, accountConcurrency = account.ID, account.Concurrency
	}
	if routed, ok := upstream.(RoutedHTTPUpstream); ok {
		return routed.DoWithTLSRoute(req, route, accountID, accountConcurrency, profile)
	}
	legacyProxyURL, err := legacyHTTPUpstreamProxyURL(route)
	if err != nil {
		return nil, err
	}
	return upstream.DoWithTLS(req, legacyProxyURL, accountID, accountConcurrency, profile)
}

func legacyHTTPUpstreamProxyURL(route platformegress.Route) (string, error) {
	if route.Mode == platformegress.ModeIPv6Pool {
		// The production upstream implements RoutedHTTPUpstream and applies the
		// configured policy. Preserve pre-egress test doubles and legacy callers
		// whose zero-value account means inherited direct while the feature is
		// disabled; any explicit or already-bound IPv6 route still fails closed.
		if route.Inherited && route.SourceIPv6 == "" {
			return "", nil
		}
		return "", fmt.Errorf("%w: HTTP upstream does not support IPv6 account routes", platformegress.ErrIPv6Unsupported)
	}
	if err := route.Validate(); err != nil {
		return "", err
	}
	return route.ProxyURL, nil
}
