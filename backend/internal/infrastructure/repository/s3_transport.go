package repository

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

// S3 configuration is request-controlled. Validate every DNS answer at the
// socket boundary and dial that exact IP, preserving the SDK's Host/signature
// and TLS server name. Environment proxies and redirects cannot bypass this.
func newS3HTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = s3DialContext(net.DefaultResolver, dialer.DialContext)
	return &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func s3DialContext(resolver platformegress.IPResolver, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolvePublicDestination(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		var failures []error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			failures = append(failures, err)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		return nil, errors.Join(failures...)
	}
}
