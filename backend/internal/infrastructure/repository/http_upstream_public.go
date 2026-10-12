package repository

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
)

const publicImageDownloadTimeout = 30 * time.Second

func (s *httpUpstreamService) doRouteWithVirtualClientKey(req *http.Request, route platformegress.Route, accountID int64, concurrency int, virtualClientKey string) (*http.Response, error) {
	if req != nil && service.HTTPUpstreamPublicDestination(req.Context()) {
		return s.doPublicDestination(req, route, concurrency)
	}
	return s.doTrustedRouteWithVirtualClientKey(req, route, accountID, concurrency, virtualClientKey)
}

func (s *httpUpstreamService) doWithTLSRouteAndVirtualClientKey(req *http.Request, route platformegress.Route, accountID int64, concurrency int, profile *tlsfingerprint.Profile, virtualClientKey string) (*http.Response, error) {
	if req != nil && service.HTTPUpstreamPublicDestination(req.Context()) {
		return s.doPublicDestination(req, route, concurrency)
	}
	return s.doTrustedWithTLSRouteAndVirtualClientKey(req, route, accountID, concurrency, profile, virtualClientKey)
}

// User-controlled destinations use fresh transports so ordinary upstream pools
// and their private-host exceptions cannot weaken image download validation.
type publicDestinationTransport struct {
	resolver  platformegress.IPResolver
	allowHTTP bool
	route     platformegress.Route
	policy    platformegress.Policy
	proxyURL  *url.URL
	settings  poolSettings
}

func (s *httpUpstreamService) doPublicDestination(req *http.Request, route platformegress.Route, concurrency int) (*http.Response, error) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return nil, fmt.Errorf("public image download requires GET or HEAD")
	}
	effective, _, _, proxyURL, err := s.normalizeEgressRoute(route)
	if err != nil {
		return nil, err
	}
	transport := &publicDestinationTransport{
		resolver:  net.DefaultResolver,
		allowHTTP: s.cfg != nil && s.cfg.Security.URLAllowlist.AllowInsecureHTTP,
		route:     effective,
		policy:    s.egressPolicy(),
		proxyURL:  proxyURL,
		settings:  s.resolvePoolSettings(s.getIsolationMode(), concurrency),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   publicImageDownloadTimeout,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if service.HTTPUpstreamRedirectsDisabled(req.Context()) {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			_, err := urlvalidator.ValidatePublicHTTPURL(next.URL.String(), transport.allowHTTP)
			return err
		},
	}
	resp, err := servertiming.Do(client, req)
	if err == nil {
		decompressResponseBody(resp)
	}
	return resp, err
}

func (t *publicDestinationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("public image URL is required")
	}
	if _, err := urlvalidator.ValidatePublicHTTPURL(req.URL.String(), t.allowHTTP); err != nil {
		return nil, err
	}
	ips, err := resolvePublicDestination(req.Context(), t.resolver, req.URL.Hostname())
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, ip := range ips {
		if t.route.Mode == platformegress.ModeIPv6Pool && !ip.Is6() {
			continue
		}
		transport, err := t.transportForIP(req.URL.Hostname())
		if err != nil {
			return nil, err
		}
		pinned := pinPublicDestination(req, ip)
		resp, err := transport.RoundTrip(pinned)
		transport.CloseIdleConnections()
		if err == nil {
			// Relative redirects must resolve against the original hostname,
			// while the proxy and socket only ever receive the validated IP.
			resp.Request = req
			return resp, nil
		}
		failures = append(failures, err)
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
	}
	if len(failures) == 0 {
		return nil, platformegress.ErrIPv6Destination
	}
	return nil, errors.Join(failures...)
}

func (t *publicDestinationTransport) transportForIP(host string) (*http.Transport, error) {
	transport, err := buildUpstreamTransportForRoute(t.settings, t.proxyURL, upstreamProtocolModeDefault, t.route, t.policy)
	if err != nil {
		return nil, err
	}
	transport.DisableKeepAlives = true
	// URL authority is pinned, but TLS must authenticate the original hostname.
	transport.TLSClientConfig = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if t.proxyURL != nil && (t.proxyURL.Scheme == "http" || t.proxyURL.Scheme == "https") {
		// CONNECT pins the proxy destination for HTTP as well as HTTPS. An
		// absolute-form HTTP proxy request would resolve Request.Host again.
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialPublicImageProxy(ctx, network, address, t.proxyURL, nil)
		}
	}
	return transport, nil
}

func pinPublicDestination(req *http.Request, ip netip.Addr) *http.Request {
	pinned := req.Clone(req.Context())
	port := req.URL.Port()
	if port == "" {
		port = "443"
		if req.URL.Scheme == "http" {
			port = "80"
		}
	}
	pinned.URL.Host = net.JoinHostPort(ip.String(), port)
	pinned.Host = req.URL.Host
	return pinned
}

func resolvePublicDestination(ctx context.Context, resolver platformegress.IPResolver, host string) ([]netip.Addr, error) {
	var ips []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{literal}
	} else {
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var err error
		ips, err = resolver.LookupNetIP(lookupCtx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve public image destination: %w", err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("public image destination has no addresses")
	}
	for i, ip := range ips {
		if err := urlvalidator.ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		ips[i] = ip.Unmap()
	}
	return ips, nil
}
