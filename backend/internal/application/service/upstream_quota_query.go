package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

const upstreamQuotaStatusTimeout = 2 * time.Second

var (
	ErrUpstreamQuotaAccountInvalid = infraerrors.BadRequest(
		"UPSTREAM_QUOTA_ACCOUNT_INVALID", "account is not a configured OpenAI API key account",
	)
	ErrUpstreamQuotaUnsupported = infraerrors.New(
		http.StatusUnprocessableEntity, "UPSTREAM_QUOTA_UNSUPPORTED", "upstream quota protocol is unsupported",
	)
	ErrUpstreamQuotaAuthFailed = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_AUTH_FAILED", "upstream rejected the account API key",
	)
	ErrUpstreamQuotaRateLimited = infraerrors.ServiceUnavailable(
		"UPSTREAM_QUOTA_RATE_LIMITED", "upstream quota query was rate limited",
	)
	ErrUpstreamQuotaTimeout = infraerrors.GatewayTimeout(
		"UPSTREAM_QUOTA_TIMEOUT", "upstream quota query timed out",
	)
	ErrUpstreamQuotaInvalidResponse = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_INVALID_RESPONSE", "upstream returned an invalid quota response",
	)
	ErrUpstreamQuotaRequestFailed = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_REQUEST_FAILED", "upstream quota request failed",
	)
	ErrUpstreamQuotaIdentityChanged = infraerrors.Conflict(
		"UPSTREAM_QUOTA_IDENTITY_CHANGED", "account identity changed during upstream quota query; retry the query",
	)
)

// UpstreamQuotaQueryResult is transient data returned only to the requesting
// administrator. It is intentionally never persisted server-side.
type UpstreamQuotaQueryResult struct {
	AccountID  int64              `json:"account_id"`
	ObservedAt time.Time          `json:"observed_at"`
	Quota      *UpstreamQuotaInfo `json:"quota"`
}

type UpstreamQuotaInfo struct {
	Provider     string                    `json:"provider"`
	Mode         string                    `json:"mode"`
	Unit         string                    `json:"unit,omitempty"`
	Remaining    *float64                  `json:"remaining,omitempty"`
	Used         *float64                  `json:"used,omitempty"`
	Total        *float64                  `json:"total,omitempty"`
	ExpiresAt    *time.Time                `json:"expires_at,omitempty"`
	Windows      []UpstreamQuotaWindow     `json:"windows,omitempty"`
	Subscription *UpstreamSubscriptionInfo `json:"subscription,omitempty"`
}

type UpstreamSubscriptionInfo struct {
	PlanName  string                `json:"plan_name"`
	Remaining *float64              `json:"remaining,omitempty"`
	Unlimited bool                  `json:"unlimited,omitempty"`
	ExpiresAt time.Time             `json:"expires_at"`
	Windows   []UpstreamQuotaWindow `json:"windows,omitempty"`
}

type UpstreamQuotaWindow struct {
	Name      string     `json:"name"`
	Used      *float64   `json:"used,omitempty"`
	Limit     *float64   `json:"limit,omitempty"`
	Remaining *float64   `json:"remaining,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

type upstreamQuotaQueryClient struct {
	upstream   HTTPUpstream
	account    *Account
	baseURL    string
	apiKey     string
	proxyURL   string
	tlsProfile *tlsfingerprint.Profile
}

type upstreamQuotaHTTPResponse struct {
	status int
	body   []byte
}

type newAPIStatusMetadata struct {
	QuotaDisplayType *string
}

// QueryAccountQuota performs a fresh, transient query. Concurrent requests for
// the same account share one operation but retain independently cancellable waits.
func (s *UpstreamBillingProbeService) QueryAccountQuota(ctx context.Context, accountID int64) (*UpstreamQuotaQueryResult, error) {
	if s == nil || s.accountRepo == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	if accountID <= 0 {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resultCh := s.quotaGroup.DoChan(strconv.FormatInt(accountID, 10), func() (any, error) {
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamBillingProbeRequestTimeout)
		defer cancel()
		return s.queryAccountQuota(opCtx, accountID)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		quota, ok := result.Val.(*UpstreamQuotaQueryResult)
		if !ok || quota == nil {
			return nil, ErrUpstreamQuotaInvalidResponse
		}
		return quota, nil
	}
}

func (s *UpstreamBillingProbeService) queryAccountQuota(ctx context.Context, accountID int64) (*UpstreamQuotaQueryResult, error) {
	select {
	case s.probeSlots <- struct{}{}:
		defer func() { <-s.probeSlots }()
	case <-ctx.Done():
		return nil, upstreamQuotaContextError(ctx)
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, upstreamQuotaRepositoryError(ctx, err)
	}
	client, err := s.newUpstreamQuotaQueryClient(account)
	if err != nil {
		return nil, err
	}
	quota, err := client.query(ctx)
	if err != nil {
		return nil, err
	}

	current, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, upstreamQuotaRepositoryError(ctx, err)
	}
	if !sameUpstreamQuotaIdentity(account, current) {
		return nil, ErrUpstreamQuotaIdentityChanged
	}
	return &UpstreamQuotaQueryResult{AccountID: accountID, ObservedAt: s.currentTime().UTC(), Quota: quota}, nil
}

func (s *UpstreamBillingProbeService) newUpstreamQuotaQueryClient(account *Account) (*upstreamQuotaQueryClient, error) {
	if !isUpstreamBillingProbeAccount(account) {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	apiKey := strings.TrimSpace(account.GetOpenAIApiKey())
	if apiKey == "" {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	baseURL, err := s.accountTestService.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return nil, ErrUpstreamQuotaRequestFailed
		}
		if account.Proxy.ID != *account.ProxyID {
			return nil, ErrUpstreamQuotaIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	return &upstreamQuotaQueryClient{
		upstream: s.accountTestService.httpUpstream, account: account, baseURL: baseURL,
		apiKey: apiKey, proxyURL: proxyURL, tlsProfile: tlsProfile,
	}, nil
}

func (c *upstreamQuotaQueryClient) query(ctx context.Context) (*UpstreamQuotaInfo, error) {
	response, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/usage"), true)
	if err != nil {
		return nil, err
	}
	if response.status == http.StatusNotFound || response.status == http.StatusMethodNotAllowed {
		return c.queryNewAPI(ctx)
	}
	if err := upstreamQuotaHTTPError(response.status, false); err != nil {
		return nil, err
	}
	quota, err := parseSub2APIUsage(response.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	return quota, nil
}

func (c *upstreamQuotaQueryClient) queryNewAPI(ctx context.Context) (*UpstreamQuotaInfo, error) {
	subscriptionResponse, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/dashboard/billing/subscription"), true)
	if err != nil {
		return nil, err
	}
	if err := upstreamQuotaHTTPError(subscriptionResponse.status, true); err != nil {
		return nil, err
	}
	subscription, err := parseNewAPISubscription(subscriptionResponse.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}

	usageResponse, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/dashboard/billing/usage"), true)
	if err != nil {
		return nil, err
	}
	if err := upstreamQuotaHTTPError(usageResponse.status, false); err != nil {
		return nil, err
	}
	usage, err := parseNewAPIUsage(usageResponse.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}

	used := *usage.TotalUsage / 100
	total := *subscription.HardLimitUSD
	remaining := total - used
	quota := &UpstreamQuotaInfo{Provider: "new_api", Mode: "quota", Remaining: float64Ptr(remaining), Used: float64Ptr(used), Total: float64Ptr(total)}
	if *subscription.AccessUntil > 0 {
		expiresAt := time.Unix(*subscription.AccessUntil, 0).UTC()
		quota.ExpiresAt = &expiresAt
	}

	statusCtx, cancel := context.WithTimeout(ctx, upstreamQuotaStatusTimeout)
	defer cancel()
	quota.Unit = c.queryNewAPIUnit(statusCtx)
	return quota, nil
}

func (c *upstreamQuotaQueryClient) queryNewAPIUnit(ctx context.Context) string {
	statusURL, err := upstreamQuotaStatusURL(c.baseURL)
	if err != nil {
		return ""
	}
	response, err := c.get(ctx, statusURL, false)
	if err != nil || response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		return ""
	}
	status, err := parseNewAPIStatusMetadata(response.body)
	if err != nil || status.QuotaDisplayType == nil {
		return ""
	}
	unit, ok := upstreamQuotaUnit(*status.QuotaDisplayType)
	if !ok {
		return ""
	}
	return unit
}

func parseNewAPIStatusMetadata(body []byte) (*newAPIStatusMetadata, error) {
	var status struct {
		Success *bool `json:"success"`
		Data    *struct {
			QuotaDisplayType *string `json:"quota_display_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &status); err != nil || status.Success == nil || !*status.Success || status.Data == nil {
		return nil, errors.New("invalid New API status response")
	}
	return &newAPIStatusMetadata{QuotaDisplayType: status.Data.QuotaDisplayType}, nil
}

func (c *upstreamQuotaQueryClient) get(ctx context.Context, endpoint string, authenticated bool) (*upstreamQuotaHTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrUpstreamQuotaRequestFailed
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	c.account.ApplyHeaderOverrides(req.Header)
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		req.Header.Del("Authorization")
	}

	resp, err := c.upstream.DoWithTLS(req, c.proxyURL, c.account.ID, c.account.Concurrency, c.tlsProfile)
	if err != nil {
		return nil, upstreamQuotaOperationError(ctx, err)
	}
	if resp == nil || resp.Body == nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, upstreamBillingProbeMaxBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, upstreamQuotaOperationError(ctx, readErr)
	}
	if int64(len(body)) > upstreamBillingProbeMaxBodyBytes {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	return &upstreamQuotaHTTPResponse{status: resp.StatusCode, body: body}, nil
}

func upstreamQuotaHTTPError(status int, unsupported bool) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUpstreamQuotaAuthFailed
	case http.StatusTooManyRequests:
		return ErrUpstreamQuotaRateLimited
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		if unsupported {
			return ErrUpstreamQuotaUnsupported
		}
	}
	return ErrUpstreamQuotaInvalidResponse
}

func upstreamQuotaOperationError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return ErrUpstreamQuotaRequestFailed
}

func upstreamQuotaRepositoryError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return err
}

func upstreamQuotaContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return ErrUpstreamQuotaRequestFailed
}

func upstreamQuotaStatusURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid upstream base URL")
	}
	parsed.User = nil
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		path = path[:strings.LastIndex(path, "/")]
	}
	parsed.Path = strings.TrimRight(path, "/") + "/api/status"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.ForceQuery = false
	return parsed.String(), nil
}

func sameUpstreamQuotaIdentity(expected, current *Account) bool {
	if !sameUpstreamProtocolIdentity(expected, current) || expected.Concurrency != current.Concurrency {
		return false
	}
	return true
}

func sameUpstreamProtocolIdentity(expected, current *Account) bool {
	if expected == nil || current == nil || expected.Platform != current.Platform || expected.Type != current.Type ||
		!reflect.DeepEqual(expected.Credentials, current.Credentials) || !sameOptionalInt64(expected.ProxyID, current.ProxyID) ||
		!sameUpstreamQuotaProxy(expected.ProxyID, expected.Proxy, current.Proxy) {
		return false
	}
	for _, key := range []string{"enable_tls_fingerprint", "tls_fingerprint_profile_id"} {
		expectedValue, expectedOK := expected.Extra[key]
		currentValue, currentOK := current.Extra[key]
		if expectedOK != currentOK || !reflect.DeepEqual(expectedValue, currentValue) {
			return false
		}
	}
	return true
}

func sameOptionalInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameUpstreamQuotaProxy(proxyID *int64, expected, current *Proxy) bool {
	if proxyID == nil {
		return true
	}
	return expected != nil && current != nil && expected.ID == *proxyID && current.ID == *proxyID &&
		expected.Protocol == current.Protocol && expected.Host == current.Host && expected.Port == current.Port &&
		expected.Username == current.Username && expected.Password == current.Password && expected.Status == current.Status
}
