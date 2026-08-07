package repository

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/httpclient"
)

const (
	tencentCaptchaEndpoint      = "https://captcha.tencentcloudapi.com"
	tencentCaptchaIntlEndpoint  = "https://captcha.intl.tencentcloudapi.com"
	tencentCaptchaAction        = "DescribeCaptchaResult"
	tencentCaptchaVersion       = "2019-07-22"
	tencentCaptchaService       = "captcha"
	tencentCaptchaAlgorithm     = "TC3-HMAC-SHA256"
	tencentCaptchaContentType   = "application/json; charset=utf-8"
	tencentCaptchaResponseLimit = 1 << 20
)

type tencentCaptchaVerifier struct {
	httpClient *http.Client
	endpoint   string
	now        func() time.Time
	initErr    error
}

type tencentCaptchaRequest struct {
	CaptchaType  uint64 `json:"CaptchaType"`
	Ticket       string `json:"Ticket"`
	UserIP       string `json:"UserIp"`
	Randstr      string `json:"Randstr"`
	CaptchaAppID uint64 `json:"CaptchaAppId"`
	AppSecretKey string `json:"AppSecretKey"`
}

type tencentCaptchaAPIError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type tencentCaptchaResponse struct {
	Response *struct {
		CaptchaCode *int64                  `json:"CaptchaCode"`
		CaptchaMsg  string                  `json:"CaptchaMsg"`
		RequestID   string                  `json:"RequestId"`
		Error       *tencentCaptchaAPIError `json:"Error"`
	} `json:"Response"`
}

func NewTencentCaptchaVerifier() service.TencentCaptchaVerifier {
	client, err := httpclient.GetClient(httpclient.Options{
		Timeout:            5 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		return &tencentCaptchaVerifier{
			endpoint: tencentCaptchaEndpoint,
			now:      time.Now,
			initErr:  fmt.Errorf("initialize restricted HTTP client: %w", err),
		}
	}
	// The pool owns the shared client. Copy it before applying this verifier's
	// fixed-endpoint redirect policy while retaining the pooled Transport.
	restricted := *client
	restricted.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &tencentCaptchaVerifier{
		httpClient: &restricted,
		endpoint:   tencentCaptchaEndpoint,
		now:        time.Now,
	}
}

func (v *tencentCaptchaVerifier) VerifyTicket(
	ctx context.Context,
	credentials service.TencentCaptchaCredentials,
	proof service.TencentCaptchaProof,
	remoteIP string,
) (*service.TencentCaptchaVerifyResponse, error) {
	if v == nil || v.initErr != nil || v.httpClient == nil {
		if v != nil && v.initErr != nil {
			return nil, v.initErr
		}
		return nil, fmt.Errorf("tencent captcha verifier is not initialized")
	}
	payload, err := json.Marshal(tencentCaptchaRequest{
		CaptchaType:  9,
		Ticket:       proof.Ticket,
		UserIP:       remoteIP,
		Randstr:      proof.Randstr,
		CaptchaAppID: credentials.AppID,
		AppSecretKey: credentials.AppSecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("encode tencent captcha request: %w", err)
	}

	endpoint := v.endpoint
	switch credentials.Endpoint {
	case "", tencentCaptchaEndpoint:
	case tencentCaptchaIntlEndpoint:
		endpoint = tencentCaptchaIntlEndpoint
	default:
		return nil, fmt.Errorf("unsupported tencent captcha endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create tencent captcha request: %w", err)
	}
	now := time.Now().UTC()
	if v.now != nil {
		now = v.now().UTC()
	}
	signTencentCaptchaRequest(req, payload, credentials, now)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send tencent captcha request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, tencentCaptchaResponseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read tencent captcha response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("tencent captcha returned HTTP %d", resp.StatusCode)
	}
	if len(body) > tencentCaptchaResponseLimit {
		return nil, fmt.Errorf("tencent captcha response exceeds %d bytes", tencentCaptchaResponseLimit)
	}
	var envelope tencentCaptchaResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode tencent captcha response: %w", err)
	}
	if envelope.Response == nil {
		return nil, fmt.Errorf("decode tencent captcha response: missing Response")
	}
	if envelope.Response.Error != nil && strings.TrimSpace(envelope.Response.Error.Code) != "" {
		return nil, fmt.Errorf("tencent captcha API error: %s", envelope.Response.Error.Code)
	}
	if envelope.Response.CaptchaCode == nil {
		return nil, fmt.Errorf("decode tencent captcha response: missing CaptchaCode")
	}
	return &service.TencentCaptchaVerifyResponse{
		CaptchaCode: *envelope.Response.CaptchaCode,
		CaptchaMsg:  envelope.Response.CaptchaMsg,
		RequestID:   envelope.Response.RequestID,
	}, nil
}

func signTencentCaptchaRequest(req *http.Request, payload []byte, credentials service.TencentCaptchaCredentials, now time.Time) {
	timestamp := fmt.Sprintf("%d", now.Unix())
	date := now.Format("2006-01-02")
	canonicalHeaders := "content-type:" + tencentCaptchaContentType + "\n" +
		"host:" + strings.ToLower(req.URL.Host) + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		req.Method,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		sha256Hex(payload),
	}, "\n")
	credentialScope := date + "/" + tencentCaptchaService + "/tc3_request"
	stringToSign := strings.Join([]string{
		tencentCaptchaAlgorithm,
		timestamp,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+credentials.CloudSecretKey), date)
	secretService := hmacSHA256(secretDate, tencentCaptchaService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := tencentCaptchaAlgorithm +
		" Credential=" + credentials.CloudSecretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", tencentCaptchaContentType)
	req.Header.Set("X-TC-Action", tencentCaptchaAction)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Version", tencentCaptchaVersion)
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
