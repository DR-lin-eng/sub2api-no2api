package egress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const maxProbeResponseBytes = 4096

type ProbeResult struct {
	SourceIPv6  string        `json:"source_ipv6"`
	ObservedIP  string        `json:"observed_ip"`
	Latency     time.Duration `json:"-"`
	LatencyMS   int64         `json:"latency_ms"`
	ProbeTarget string        `json:"probe_target"`
}

func Probe(ctx context.Context, route Route, policy Policy, target string, timeout time.Duration) (*ProbeResult, error) {
	effective, err := ApplyPolicy(route, policy)
	if err != nil {
		return nil, err
	}
	if effective.Mode != ModeIPv6Pool {
		return nil, fmt.Errorf("%w: probe requires an IPv6 pool route", ErrInvalidRoute)
	}
	dialContext, err := NewDialContext(effective, policy, DialerOptions{Timeout: timeout})
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext:         dialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: timeout,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(target), nil)
	if err != nil {
		return nil, fmt.Errorf("create IPv6 egress probe request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9")
	started := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(started)
	if err != nil {
		return nil, fmt.Errorf("IPv6 egress probe request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("IPv6 egress probe returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read IPv6 egress probe response: %w", err)
	}
	if len(body) > maxProbeResponseBytes {
		return nil, fmt.Errorf("IPv6 egress probe response exceeds %d bytes", maxProbeResponseBytes)
	}
	observed, err := extractObservedIPv6(body)
	if err != nil {
		return nil, err
	}
	source := netip.MustParseAddr(effective.SourceIPv6).Unmap()
	if observed != source {
		return nil, fmt.Errorf("IPv6 egress probe observed %s, expected %s", observed, source)
	}
	return &ProbeResult{
		SourceIPv6:  source.String(),
		ObservedIP:  observed.String(),
		Latency:     latency,
		LatencyMS:   latency.Milliseconds(),
		ProbeTarget: req.URL.Hostname(),
	}, nil
}

func extractObservedIPv6(body []byte) (netip.Addr, error) {
	raw := strings.TrimSpace(string(body))
	var payload struct {
		IP     string `json:"ip"`
		Origin string `json:"origin"`
	}
	if strings.HasPrefix(raw, "{") && json.Unmarshal(body, &payload) == nil {
		if strings.TrimSpace(payload.IP) != "" {
			raw = payload.IP
		} else if strings.TrimSpace(payload.Origin) != "" {
			raw = payload.Origin
		}
	}
	if before, _, found := strings.Cut(raw, ","); found {
		raw = before
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || !addr.Is6() || addr.Is4In6() {
		return netip.Addr{}, fmt.Errorf("IPv6 egress probe returned invalid IPv6 address %q", raw)
	}
	return addr.Unmap(), nil
}
