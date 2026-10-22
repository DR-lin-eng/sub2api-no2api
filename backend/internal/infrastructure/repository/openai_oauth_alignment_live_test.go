package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

type alignmentLiveUpstream struct {
	service.HTTPUpstream
	statuses         []int
	proofStatus      int
	deviceError      bool
	integrityUpdates int
}

func (u *alignmentLiveUpstream) observed(req *http.Request, response *http.Response) {
	if response != nil {
		u.statuses = append(u.statuses, response.StatusCode)
		if response.Header.Get("X-OAI-IS-Update") != "" {
			u.integrityUpdates++
		}
	}
	var proof struct {
		Status int    `json:"s"`
		Token  string `json:"t"`
	}
	u.proofStatus = -1
	if json.Unmarshal([]byte(req.Header.Get("x-oai-attestation")), &proof) == nil {
		u.proofStatus = proof.Status
		if strings.HasPrefix(proof.Token, "v1.") {
			payload, _ := base64.RawURLEncoding.DecodeString(proof.Token[3:])
			u.deviceError = bytes.Contains(payload, []byte("error_code"))
		}
	}
}

func (u *alignmentLiveUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	resp, err := u.HTTPUpstream.Do(req, proxy, id, concurrency)
	u.observed(req, resp)
	return resp, err
}

func (u *alignmentLiveUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	resp, err := u.HTTPUpstream.DoWithTLS(req, proxy, id, concurrency, profile)
	u.observed(req, resp)
	return resp, err
}

// This opt-in test reads the user's supplied export in memory. It never
// imports an account into a database, rotates refresh tokens or logs secrets.
func TestOpenAIOAuthAlignmentLiveSuppliedAccount(t *testing.T) {
	path := os.Getenv("SUB2API_TEST_OAUTH_ACCOUNT_FILE")
	if path == "" {
		t.Skip("explicit live account file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read supplied account export")
	}
	var exported struct {
		Accounts []struct {
			Platform, Type     string
			Credentials, Extra map[string]any
			ProxyKey           string `json:"proxy_key"`
		}
		Proxies []struct {
			Protocol, Host, Username, Password string
			Port                               int
			ProxyKey                           string `json:"proxy_key"`
		}
	}
	if json.Unmarshal(data, &exported) != nil || len(exported.Accounts) != 1 {
		t.Fatal("expected one account export")
	}
	x := exported.Accounts[0]
	account := &service.Account{ID: 701, Platform: x.Platform, Type: x.Type, Credentials: x.Credentials, Extra: x.Extra, Concurrency: 1}
	if !account.IsOpenAIOAuth() || account.GetOpenAIAccessToken() == "" {
		t.Fatal("supplied OAuth token missing")
	}
	if ua := os.Getenv("SUB2API_TEST_OAUTH_UA"); ua != "" {
		account.Credentials["user_agent"] = ua
	}
	if x.ProxyKey != "" && os.Getenv("SUB2API_TEST_OAUTH_DIRECT") != "1" {
		for _, p := range exported.Proxies {
			if p.ProxyKey == x.ProxyKey {
				id := int64(701)
				account.ProxyID = &id
				account.Proxy = &service.Proxy{ID: id, Protocol: p.Protocol, Host: p.Host, Port: p.Port, Username: p.Username, Password: p.Password}
			}
		}
		if account.Proxy == nil {
			t.Fatal("exported proxy association missing")
		}
	}
	cfg := &config.Config{}
	cfg.Gateway.ResponseHeaderTimeout = 45
	wsRequested := os.Getenv("SUB2API_TEST_OAUTH_WS") == "1"
	if wsRequested {
		cfg.Gateway.OpenAIWS.Enabled = true
		cfg.Gateway.OpenAIWS.OAuthEnabled = true
		cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
		cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
		cfg.Gateway.OpenAIWS.IngressModeDefault = "ctx_pool"
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		account.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
		account.Extra["openai_oauth_responses_websockets_v2_mode"] = "ctx_pool"
		account.Extra["codex_prewarm_continuation_enabled"] = false
	}
	// Reflection allows the same test source to execute against the pristine
	// baseline, which has no proof enable field, for a literal before/after run.
	if field := reflect.ValueOf(&cfg.Gateway).Elem().FieldByName("OpenAIOAuthAttestationEnabled"); field.IsValid() {
		field.SetBool(true)
	}
	upstream := &alignmentLiveUpstream{HTTPUpstream: NewHTTPUpstream(cfg, nil), proofStatus: -1}
	gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	gin.SetMode(gin.TestMode)
	streams := []bool{false, true}
	if wsRequested {
		streams = []bool{true}
	}
	for _, stream := range streams {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("Accept", "application/json")
		body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","instructions":"Reply only OK.","input":"Reply OK.","stream":%t,"store":false,"reasoning":{"effort":"none"}}`, stream))
		result, forwardErr := gateway.Forward(ctx, c, account, body)
		cancel()
		// Print only outcome classes. Upstream bodies, account claims, proxy
		// credentials and opaque integrity/proof values are deliberately omitted.
		succeeded := forwardErr == nil && result != nil
		usedWS := result != nil && result.OpenAIWSMode
		fmt.Printf("LIVE_ALIGNMENT stream=%t ws_requested=%t ws_used=%t proxy=%t upstream_statuses=%v downstream_status=%d completed=%t proof_status=%d device_error_reported=%t integrity_updates=%d output_bytes=%d\n", stream, wsRequested, usedWS, account.Proxy != nil, upstream.statuses, c.Writer.Status(), succeeded, upstream.proofStatus, upstream.deviceError, upstream.integrityUpdates, recorder.Body.Len())
		if !succeeded {
			code := "forward_failed"
			if strings.Contains(recorder.Body.String(), "token_expired") {
				code = "token_expired"
			}
			t.Fatalf("live OAuth request failed (%s); secret response omitted", code)
		}
		if wsRequested && !usedWS {
			t.Fatal("live WS request fell back to HTTP; not counted as WS verification")
		}
	}
}
