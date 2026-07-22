package ip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ResolutionModeAutoCompat   = "auto_compat"
	ResolutionModeTrustedProxy = "trusted_proxy"
	ResolutionModeDirect       = "direct"

	MaxTrustedProxyPrefixes = 64
	maxForwardedForBytes    = 4 << 10
	maxForwardedForHops     = 16

	cloudflareRangesURL          = "https://api.cloudflare.com/client/v4/ips"
	cloudflareRefreshInterval    = 24 * time.Hour
	cloudflareRefreshTimeout     = 5 * time.Second
	cloudflareResponseLimitBytes = 256 << 10
	maxCloudflarePrefixes        = 256
	headerCFConnectingIP         = "Cf-Connecting-Ip"
	headerXForwardedFor          = "X-Forwarded-For"
	headerXRealIP                = "X-Real-Ip"
)

type clientIPResultContextKey struct{}

var embeddedCloudflarePrefixes = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

type ClientIPSource uint8

const (
	ClientIPSourceUnknown ClientIPSource = iota
	ClientIPSourceDirect
	ClientIPSourceCloudflare
	ClientIPSourceForwardedFor
	ClientIPSourceRealIP
	clientIPSourceCount
)

func (s ClientIPSource) String() string {
	switch s {
	case ClientIPSourceDirect:
		return "direct"
	case ClientIPSourceCloudflare:
		return "cloudflare"
	case ClientIPSourceForwardedFor:
		return "x_forwarded_for"
	case ClientIPSourceRealIP:
		return "x_real_ip"
	default:
		return "unknown"
	}
}

// ClientIPResult is immutable after it is attached to a request context.
type ClientIPResult struct {
	Addr   netip.Addr
	IP     string
	Source ClientIPSource
}

type ResolutionStatus struct {
	Mode                    string     `json:"mode"`
	CustomPrefixCount       int        `json:"custom_prefix_count"`
	StaticPrefixCount       int        `json:"static_prefix_count"`
	CloudflarePrefixCount   int        `json:"cloudflare_prefix_count"`
	CloudflareRangesSource  string     `json:"cloudflare_ranges_source"`
	CloudflareLastSuccessAt *time.Time `json:"cloudflare_last_success_at"`
}

type ResolverMetrics struct {
	Resolutions uint64
	BySource    map[string]uint64
}

type resolverSnapshot struct {
	mode string

	trustedV4 []netip.Prefix
	trustedV6 []netip.Prefix
	cloudV4   []netip.Prefix
	cloudV6   []netip.Prefix

	customPrefixes []string
	status         ResolutionStatus
}

// Resolver keeps all request-path state in an immutable atomic snapshot.
// updateMu is only used by settings and Cloudflare refresh operations.
type Resolver struct {
	snapshot atomic.Pointer[resolverSnapshot]
	updateMu sync.Mutex

	mode            string
	staticPrefixes  []netip.Prefix
	customPrefixes  []netip.Prefix
	customCanonical []string
	cloudPrefixes   []netip.Prefix
	cloudSource     string
	cloudLastOK     time.Time

	httpClient *http.Client
	rangesURL  string

	started  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	sourceCounts [clientIPSourceCount]atomic.Uint64
}

func NewResolver(staticTrustedProxies []string) (*Resolver, error) {
	staticPrefixes, _, err := compileTrustedPrefixes(staticTrustedProxies, 0)
	if err != nil {
		return nil, fmt.Errorf("compile server trusted proxies: %w", err)
	}
	cloudPrefixes, _, err := compileTrustedPrefixes(embeddedCloudflarePrefixes, maxCloudflarePrefixes)
	if err != nil {
		return nil, fmt.Errorf("compile embedded Cloudflare prefixes: %w", err)
	}
	r := &Resolver{
		mode:           ResolutionModeAutoCompat,
		staticPrefixes: staticPrefixes,
		cloudPrefixes:  cloudPrefixes,
		cloudSource:    "embedded",
		httpClient: &http.Client{
			Timeout: cloudflareRefreshTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("cloudflare ranges endpoint redirect refused")
			},
		},
		rangesURL: cloudflareRangesURL,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	r.rebuildSnapshotLocked()
	return r, nil
}

func ValidateResolutionMode(mode string) error {
	switch strings.TrimSpace(mode) {
	case ResolutionModeAutoCompat, ResolutionModeTrustedProxy, ResolutionModeDirect:
		return nil
	default:
		return fmt.Errorf("unsupported client IP resolution mode %q", mode)
	}
}

func ValidateTrustedProxies(values []string) error {
	_, _, err := compileTrustedPrefixes(values, MaxTrustedProxyPrefixes)
	return err
}

func NormalizeTrustedProxies(values []string) ([]string, error) {
	_, canonical, err := compileTrustedPrefixes(values, MaxTrustedProxyPrefixes)
	return canonical, err
}

func (r *Resolver) Configure(mode string, trustedProxies []string) error {
	if r == nil {
		return errors.New("client IP resolver is nil")
	}
	mode = strings.TrimSpace(mode)
	if err := ValidateResolutionMode(mode); err != nil {
		return err
	}
	prefixes, canonical, err := compileTrustedPrefixes(trustedProxies, MaxTrustedProxyPrefixes)
	if err != nil {
		return err
	}

	r.updateMu.Lock()
	r.mode = mode
	r.customPrefixes = prefixes
	r.customCanonical = canonical
	r.rebuildSnapshotLocked()
	r.updateMu.Unlock()
	return nil
}

func (r *Resolver) CurrentConfiguration() (string, []string) {
	if r == nil {
		return ResolutionModeAutoCompat, nil
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return ResolutionModeAutoCompat, nil
	}
	return snapshot.mode, append([]string(nil), snapshot.customPrefixes...)
}

func (r *Resolver) Status() ResolutionStatus {
	if r == nil {
		return ResolutionStatus{Mode: ResolutionModeAutoCompat, CloudflareRangesSource: "embedded"}
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return ResolutionStatus{Mode: ResolutionModeAutoCompat, CloudflareRangesSource: "embedded"}
	}
	status := snapshot.status
	if status.CloudflareLastSuccessAt != nil {
		lastOK := *status.CloudflareLastSuccessAt
		status.CloudflareLastSuccessAt = &lastOK
	}
	return status
}

func (r *Resolver) Metrics() ResolverMetrics {
	metrics := ResolverMetrics{BySource: make(map[string]uint64, int(clientIPSourceCount)-1)}
	if r == nil {
		return metrics
	}
	for source := ClientIPSourceDirect; source < clientIPSourceCount; source++ {
		count := r.sourceCounts[source].Load()
		metrics.BySource[source.String()] = count
		metrics.Resolutions += count
	}
	return metrics
}

func (r *Resolver) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		result := r.ResolveRequest(c.Request)
		if c.Request != nil {
			ctx := context.WithValue(c.Request.Context(), clientIPResultContextKey{}, &result)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}

func ResultFromContext(c *gin.Context) (*ClientIPResult, bool) {
	if c == nil {
		return nil, false
	}
	if c.Request == nil {
		return nil, false
	}
	result, ok := c.Request.Context().Value(clientIPResultContextKey{}).(*ClientIPResult)
	return result, ok && result != nil
}

func (r *Resolver) ResolveRequest(req *http.Request) ClientIPResult {
	if r == nil || req == nil {
		return ClientIPResult{}
	}
	peer, ok := parsePeerAddr(req.RemoteAddr)
	if !ok {
		return ClientIPResult{}
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil || snapshot.mode == ResolutionModeDirect {
		return r.recordResult(peer, ClientIPSourceDirect)
	}
	if !hasForwardingHeaders(req.Header) {
		return r.recordResult(peer, ClientIPSourceDirect)
	}

	peerIsCloudflare := !isInfrastructureAddress(peer) && containsPrefix(peer, snapshot.cloudV4, snapshot.cloudV6)
	if peerIsCloudflare {
		if addr, ok := parseSingleIPHeader(req.Header, headerCFConnectingIP); ok {
			return r.recordResult(addr, ClientIPSourceCloudflare)
		}
	}

	if !isTrustedPeer(peer, snapshot) {
		return r.recordResult(peer, ClientIPSourceDirect)
	}

	if value, ok := singleHeaderValue(req.Header, headerXForwardedFor); ok && value != "" {
		if addr, valid := resolveForwardedFor(value, snapshot); valid {
			return r.recordResult(addr, ClientIPSourceForwardedFor)
		}
	}
	if addr, ok := parseSingleIPHeader(req.Header, headerXRealIP); ok {
		return r.recordResult(addr, ClientIPSourceRealIP)
	}
	if addr, ok := parseSingleIPHeader(req.Header, headerCFConnectingIP); ok {
		return r.recordResult(addr, ClientIPSourceCloudflare)
	}
	return r.recordResult(peer, ClientIPSourceDirect)
}

func hasForwardingHeaders(header http.Header) bool {
	return len(header[headerCFConnectingIP]) > 0 ||
		len(header[headerXForwardedFor]) > 0 ||
		len(header[headerXRealIP]) > 0
}

func (r *Resolver) recordResult(addr netip.Addr, source ClientIPSource) ClientIPResult {
	addr = addr.Unmap()
	if source > ClientIPSourceUnknown && source < clientIPSourceCount {
		r.sourceCounts[source].Add(1)
	}
	return ClientIPResult{Addr: addr, IP: addr.String(), Source: source}
}

func resolveForwardedFor(value string, snapshot *resolverSnapshot) (netip.Addr, bool) {
	if len(value) == 0 || len(value) > maxForwardedForBytes {
		return netip.Addr{}, false
	}
	firstComma := strings.IndexByte(value, ',')
	if firstComma < 0 {
		return parseHeaderAddr(value)
	}
	commaCount := 1
	for offset := firstComma + 1; offset < len(value); {
		next := strings.IndexByte(value[offset:], ',')
		if next < 0 {
			break
		}
		commaCount++
		if commaCount >= maxForwardedForHops {
			return netip.Addr{}, false
		}
		offset += next + 1
	}
	end := len(value)
	var leftmost netip.Addr
	for hops := 1; ; hops++ {
		if hops > maxForwardedForHops {
			return netip.Addr{}, false
		}
		start := strings.LastIndexByte(value[:end], ',')
		tokenStart := start + 1
		token := strings.TrimSpace(value[tokenStart:end])
		addr, ok := parseHeaderAddr(token)
		if !ok {
			return netip.Addr{}, false
		}
		leftmost = addr
		if !isTrustedPeer(addr, snapshot) {
			return addr, true
		}
		if start < 0 {
			return leftmost, true
		}
		end = start
	}
}

func isTrustedPeer(addr netip.Addr, snapshot *resolverSnapshot) bool {
	if !addr.IsValid() || snapshot == nil {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return true
	}
	if snapshot.mode == ResolutionModeAutoCompat && isInfrastructureAddress(addr) {
		return true
	}
	return containsPrefix(addr, snapshot.cloudV4, snapshot.cloudV6) ||
		containsPrefix(addr, snapshot.trustedV4, snapshot.trustedV6)
}

func isInfrastructureAddress(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func containsPrefix(addr netip.Addr, v4, v6 []netip.Prefix) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	prefixes := v6
	if addr.Is4() {
		prefixes = v4
	}
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parsePeerAddr(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if addrPort, err := netip.ParseAddrPort(value); err == nil {
		return addrPort.Addr().Unmap(), true
	}
	return parseHeaderAddr(value)
}

func parseHeaderAddr(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '[' && value[len(value)-1] == ']' {
		value = value[1 : len(value)-1]
	}
	addr, err := netip.ParseAddr(value)
	if err != nil || !addr.IsValid() {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func singleHeaderValue(header http.Header, name string) (string, bool) {
	values := header[name]
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func parseSingleIPHeader(header http.Header, name string) (netip.Addr, bool) {
	value, ok := singleHeaderValue(header, name)
	if !ok {
		return netip.Addr{}, false
	}
	return parseHeaderAddr(value)
}

func compileTrustedPrefixes(values []string, max int) ([]netip.Prefix, []string, error) {
	if max > 0 && len(values) > max {
		return nil, nil, fmt.Errorf("trusted proxy prefix count exceeds %d", max)
	}
	prefixes := make([]netip.Prefix, 0, len(values))
	canonical := make([]string, 0, len(values))
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, nil, errors.New("trusted proxy prefix must not be empty")
		}
		prefix, err := parsePrefix(value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid trusted proxy %q: %w", value, err)
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
		canonical = append(canonical, prefix.String())
	}
	return prefixes, canonical, nil
}

func parsePrefix(value string) (netip.Prefix, error) {
	if !strings.Contains(value, "/") {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		addr = addr.Unmap()
		bits := 128
		if addr.Is4() {
			bits = 32
		}
		return netip.PrefixFrom(addr, bits), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr := prefix.Addr()
	bits := prefix.Bits()
	if addr.Is4In6() {
		if bits < 96 {
			return netip.Prefix{}, errors.New("IPv4-mapped prefix must be at least /96")
		}
		addr = addr.Unmap()
		bits -= 96
	}
	return netip.PrefixFrom(addr, bits).Masked(), nil
}

func splitPrefixes(prefixes []netip.Prefix) ([]netip.Prefix, []netip.Prefix) {
	v4 := make([]netip.Prefix, 0, len(prefixes))
	v6 := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix.Addr().Is4() {
			v4 = append(v4, prefix)
		} else {
			v6 = append(v6, prefix)
		}
	}
	return v4, v6
}

func (r *Resolver) rebuildSnapshotLocked() {
	trusted := make([]netip.Prefix, 0, len(r.staticPrefixes)+len(r.customPrefixes))
	trusted = append(trusted, r.staticPrefixes...)
	trusted = append(trusted, r.customPrefixes...)
	trustedV4, trustedV6 := splitPrefixes(trusted)
	cloudV4, cloudV6 := splitPrefixes(r.cloudPrefixes)
	status := ResolutionStatus{
		Mode:                   r.mode,
		CustomPrefixCount:      len(r.customPrefixes),
		StaticPrefixCount:      len(r.staticPrefixes),
		CloudflarePrefixCount:  len(r.cloudPrefixes),
		CloudflareRangesSource: r.cloudSource,
	}
	if !r.cloudLastOK.IsZero() {
		lastOK := r.cloudLastOK
		status.CloudflareLastSuccessAt = &lastOK
	}
	r.snapshot.Store(&resolverSnapshot{
		mode:           r.mode,
		trustedV4:      trustedV4,
		trustedV6:      trustedV6,
		cloudV4:        cloudV4,
		cloudV6:        cloudV6,
		customPrefixes: append([]string(nil), r.customCanonical...),
		status:         status,
	})
}

func (r *Resolver) Start() {
	if r == nil || !r.started.CompareAndSwap(false, true) {
		return
	}
	go r.refreshLoop()
}

func (r *Resolver) Stop() {
	if r == nil || !r.started.Load() {
		return
	}
	r.stopOnce.Do(func() { close(r.stopCh) })
	<-r.doneCh
}

func (r *Resolver) refreshLoop() {
	defer close(r.doneCh)
	if err := r.refreshCloudflareRanges(context.Background()); err != nil {
		log.Printf("client IP resolver: Cloudflare range refresh failed, keeping last-known-good ranges: %v", err)
	}
	ticker := time.NewTicker(cloudflareRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := r.refreshCloudflareRanges(context.Background()); err != nil {
				log.Printf("client IP resolver: Cloudflare range refresh failed, keeping last-known-good ranges: %v", err)
			}
		case <-r.stopCh:
			return
		}
	}
}

type cloudflareRangesResponse struct {
	Success bool `json:"success"`
	Result  struct {
		IPv4CIDRs []string `json:"ipv4_cidrs"`
		IPv6CIDRs []string `json:"ipv6_cidrs"`
	} `json:"result"`
}

func (r *Resolver) refreshCloudflareRanges(parent context.Context) error {
	if r == nil {
		return errors.New("client IP resolver is nil")
	}
	ctx, cancel := context.WithTimeout(parent, cloudflareRefreshTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.rangesURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API/client-ip-resolver")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloudflare ranges endpoint returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cloudflareResponseLimitBytes+1))
	if err != nil {
		return err
	}
	if len(body) > cloudflareResponseLimitBytes {
		return fmt.Errorf("cloudflare ranges response exceeds %d bytes", cloudflareResponseLimitBytes)
	}
	var payload cloudflareRangesResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode Cloudflare ranges: %w", err)
	}
	if !payload.Success || len(payload.Result.IPv4CIDRs) == 0 || len(payload.Result.IPv6CIDRs) == 0 {
		return errors.New("cloudflare ranges response is incomplete")
	}
	if len(payload.Result.IPv4CIDRs)+len(payload.Result.IPv6CIDRs) > maxCloudflarePrefixes {
		return fmt.Errorf("cloudflare prefix count exceeds %d", maxCloudflarePrefixes)
	}
	v4Prefixes, _, err := compileTrustedPrefixes(payload.Result.IPv4CIDRs, maxCloudflarePrefixes)
	if err != nil {
		return fmt.Errorf("validate Cloudflare IPv4 ranges: %w", err)
	}
	for _, prefix := range v4Prefixes {
		if err := validateCloudflarePrefix(prefix, true); err != nil {
			return err
		}
	}
	v6Prefixes, _, err := compileTrustedPrefixes(payload.Result.IPv6CIDRs, maxCloudflarePrefixes)
	if err != nil {
		return fmt.Errorf("validate Cloudflare IPv6 ranges: %w", err)
	}
	for _, prefix := range v6Prefixes {
		if err := validateCloudflarePrefix(prefix, false); err != nil {
			return err
		}
	}
	prefixes := make([]netip.Prefix, 0, len(v4Prefixes)+len(v6Prefixes))
	prefixes = append(prefixes, v4Prefixes...)
	prefixes = append(prefixes, v6Prefixes...)

	r.updateMu.Lock()
	r.cloudPrefixes = prefixes
	r.cloudSource = "refreshed"
	r.cloudLastOK = time.Now().UTC()
	r.rebuildSnapshotLocked()
	r.updateMu.Unlock()
	return nil
}

func validateCloudflarePrefix(prefix netip.Prefix, wantIPv4 bool) error {
	addr := prefix.Addr()
	if addr.Is4() != wantIPv4 {
		return errors.New("cloudflare range list contains an address-family mismatch")
	}
	minimumBits := 16
	if wantIPv4 {
		minimumBits = 8
	}
	if prefix.Bits() < minimumBits || !addr.IsGlobalUnicast() || isInfrastructureAddress(addr) {
		return fmt.Errorf("cloudflare range %q is not a bounded public prefix", prefix)
	}
	return nil
}
