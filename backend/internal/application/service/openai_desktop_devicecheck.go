package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
	"golang.org/x/net/publicsuffix"
)

var errOpenAIDeviceCheckEndpoint = errors.New("DeviceCheck is only allowed on the desktop challenge or messaging endpoint")
var openAIDeviceCheckMessagesPath = regexp.MustCompile(`^/(?:backend-api|api)/messaging/rooms/[^/]+/messages$`)

type openAIDeviceCookieBinding struct {
	jar       *cookiejar.Jar
	expiresAt time.Time
}

func isOpenAIDeviceCheckEndpoint(method, path string) bool {
	return strings.HasSuffix(path, "/ios/attestation_challenge") ||
		(method == http.MethodPost && openAIDeviceCheckMessagesPath.MatchString(path))
}

func (s *OpenAIGatewayService) buildOpenAIDeviceCheckHeaders(ctx context.Context, method, path string) (http.Header, error) {
	if !isOpenAIDeviceCheckEndpoint(method, path) {
		return nil, errOpenAIDeviceCheckEndpoint
	}
	if s == nil {
		return nil, liveattestation.ErrUnsupportedPlatform
	}
	scope, bound := liveattestation.RequestScopeFromContext(ctx)
	if !bound {
		return nil, liveattestation.ErrScopeMismatch
	}
	provider, ok := liveattestation.ProviderForScope(s.liveAttestation, scope.Key).(liveattestation.DeviceCheckTokenProvider)
	if !ok {
		return nil, liveattestation.ErrUnsupportedPlatform
	}
	token, err := provider.GenerateDeviceCheckToken(ctx)
	if err != nil {
		return nil, err
	}
	return liveattestation.BuildDeviceCheckHeaders(token, "com.openai.codex")
}

// The wire is the raw Apple token for x-sentinel-dc and /devicecheck. It is
// deliberately distinct from the CBOR v1 token wrapped in x-oai-attestation.
func (s *OpenAIGatewayService) attachDesktopDeviceCheck(req *http.Request, account, owner *Account) error {
	deleteOpenAIHeaderEqualFold(req.Header, "x-sentinel-dc")
	deleteOpenAIHeaderEqualFold(req.Header, "OAI-Package-Name")
	if s.cfg == nil || owner == nil || owner.IsOpenAIPersonalAccessToken() || owner.IsOpenAIAgentIdentity() || !s.cfg.Gateway.OpenAIDesktopDeviceCheckEnabled || openAIDesktopProofOrigin(req.URL.String()) == "" || !isOpenAIDeviceCheckEndpoint(req.Method, req.URL.Path) {
		return nil
	}
	headers, err := s.buildOpenAIDeviceCheckHeaders(req.Context(), req.Method, req.URL.Path)
	if err != nil {
		// Provider error text is not exported: native failures can carry device
		// details. The caller only needs to know no valid proof was produced.
		return errors.New("native DeviceCheck generation is unavailable")
	}
	scope, _ := liveattestation.RequestScopeFromContext(req.Context())
	deleteOpenAIHeaderEqualFold(req.Header, "oai-did")
	req.Header.Set("oai-did", scope.InstallationID)
	if req.Method == http.MethodPost && openAIDeviceCheckMessagesPath.MatchString(req.URL.Path) {
		var token struct {
			Token string `json:"token"`
		}
		if json.Unmarshal([]byte(headers.Get("x-sentinel-dc")), &token) != nil {
			return errors.New("invalid native DeviceCheck projection")
		}
		jar, err := s.ensureDesktopDeviceCookie(req, account, owner, token.Token)
		if err != nil {
			return err
		}
		deleteOpenAIHeaderEqualFold(req.Header, "Cookie")
		for _, cookie := range jar.Cookies(req.URL) {
			req.AddCookie(cookie)
		}
	}
	for name, values := range headers {
		req.Header[name] = values
	}
	return nil
}

func (s *OpenAIGatewayService) ensureDesktopDeviceCookie(req *http.Request, account, owner *Account, token string) (*cookiejar.Jar, error) {
	scope := openAIProofScope(req.Context(), account, owner, openAIDesktopProofOrigin(req.URL.String()), req.Header)
	key := scope.Key
	value, err, _ := s.openAIDeviceCookieSF.Do(key, func() (any, error) {
		s.openAIDeviceCookieMu.Lock()
		binding := s.openAIDeviceCookies[key]
		s.openAIDeviceCookieMu.Unlock()
		if binding.jar != nil && time.Now().Add(time.Minute).Before(binding.expiresAt) && len(binding.jar.Cookies(req.URL)) > 0 {
			return binding.jar, nil
		}
		body, err := json.Marshal(map[string]string{"bundle_id": "com.openai.codex", "device_token": token})
		if err != nil {
			return nil, err
		}
		registrationURL := *req.URL
		prefix := "/backend-api"
		if strings.HasPrefix(req.URL.Path, "/api/") {
			prefix = "/api"
		}
		registrationURL.Path, registrationURL.RawPath, registrationURL.RawQuery, registrationURL.Fragment = prefix+"/devicecheck", "", "", ""
		ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
		defer cancel()
		ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
		registration, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationURL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		// The registration endpoint receives the raw token and the same selected
		// principal/surface. It does not inherit the messaging body or cookie.
		for _, name := range []string{"Authorization", "ChatGPT-Account-Id", "X-OpenAI-Fedramp", "originator", "User-Agent", "oai-did"} {
			if v := req.Header.Get(name); v != "" {
				registration.Header.Set(name, v)
			}
		}
		registration.Header.Set("Content-Type", "application/json")
		registration.Header.Set("Accept", "application/json")
		response, err := s.doAccountHTTPUpstream(registration, resolveAccountProxyURL(account), account)
		if err != nil {
			return nil, errors.New("DeviceCheck cookie registration transport failed")
		}
		if response == nil {
			return nil, errors.New("DeviceCheck cookie registration returned no response")
		}
		defer func() { _ = response.Body.Close() }()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, errors.New("DeviceCheck cookie registration was rejected")
		}
		jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		if err != nil {
			return nil, err
		}
		expiresAt := time.Now().Add(24 * time.Hour)
		for _, cookie := range response.Cookies() {
			if cookie.Name != "_devicecheck" || cookie.Valid() != nil || cookie.MaxAge < 0 {
				continue
			}
			jar.SetCookies(&registrationURL, []*http.Cookie{cookie})
			if !cookie.Expires.IsZero() && cookie.Expires.Before(expiresAt) {
				expiresAt = cookie.Expires
			}
			if cookie.MaxAge > 0 {
				expires := time.Now().Add(time.Duration(cookie.MaxAge) * time.Second)
				if expires.Before(expiresAt) {
					expiresAt = expires
				}
			}
		}
		if len(jar.Cookies(req.URL)) == 0 {
			return nil, errors.New("DeviceCheck registration returned no valid device cookie")
		}
		s.openAIDeviceCookieMu.Lock()
		defer s.openAIDeviceCookieMu.Unlock()
		if s.openAIDeviceCookies == nil {
			s.openAIDeviceCookies = make(map[string]openAIDeviceCookieBinding)
		}
		for oldKey, entry := range s.openAIDeviceCookies {
			if time.Now().After(entry.expiresAt) {
				delete(s.openAIDeviceCookies, oldKey)
			}
		}
		if len(s.openAIDeviceCookies) < openAIIntegrityLocalMax {
			s.openAIDeviceCookies[key] = openAIDeviceCookieBinding{jar: jar, expiresAt: expiresAt}
		}
		return jar, nil
	})
	if err != nil {
		return nil, err
	}
	jar, ok := value.(*cookiejar.Jar)
	if !ok {
		return nil, errors.New("invalid DeviceCheck cookie state")
	}
	return jar, nil
}
