package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/httpclient"
)

const (
	aliyunCaptchaAction        = "VerifyIntelligentCaptcha"
	aliyunCaptchaVersion       = "2023-03-05"
	aliyunCaptchaAlgorithm     = "ACS3-HMAC-SHA256"
	aliyunCaptchaContentType   = "application/x-www-form-urlencoded"
	aliyunCaptchaResponseLimit = 1 << 20
)

type aliyunCaptchaVerifier struct {
	httpClient *http.Client
	scheme     string
	now        func() time.Time
	nonce      func() (string, error)
	initErr    error
}

type aliyunCaptchaResponse struct {
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	RequestID string `json:"RequestId"`
	Success   bool   `json:"Success"`
	Result    *struct {
		VerifyResult bool   `json:"VerifyResult"`
		VerifyCode   string `json:"VerifyCode"`
	} `json:"Result"`
}

func NewAliyunCaptchaVerifier() service.AliyunCaptchaVerifier {
	client, err := httpclient.GetClient(httpclient.Options{
		Timeout:            5 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		return &aliyunCaptchaVerifier{
			scheme:  "https",
			now:     time.Now,
			nonce:   newAliyunCaptchaNonce,
			initErr: fmt.Errorf("initialize restricted HTTP client: %w", err),
		}
	}
	restricted := *client
	restricted.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &aliyunCaptchaVerifier{
		httpClient: &restricted,
		scheme:     "https",
		now:        time.Now,
		nonce:      newAliyunCaptchaNonce,
	}
}

func (v *aliyunCaptchaVerifier) VerifyCaptcha(ctx context.Context, credentials service.AliyunCaptchaCredentials, captchaVerifyParam string) (*service.AliyunCaptchaVerifyResult, error) {
	if v == nil || v.initErr != nil || v.httpClient == nil {
		if v != nil && v.initErr != nil {
			return nil, v.initErr
		}
		return nil, fmt.Errorf("aliyun captcha verifier is not initialized")
	}
	if strings.TrimSpace(credentials.AccessKeyID) == "" || strings.TrimSpace(credentials.AccessKeySecret) == "" || strings.TrimSpace(credentials.SceneID) == "" || strings.TrimSpace(credentials.Endpoint) == "" {
		return nil, fmt.Errorf("aliyun captcha credentials are incomplete")
	}
	if strings.TrimSpace(captchaVerifyParam) == "" {
		return nil, fmt.Errorf("aliyun captcha verify parameter is empty")
	}

	body := url.Values{
		"CaptchaVerifyParam": {captchaVerifyParam},
		"SceneId":            {credentials.SceneID},
	}.Encode()
	endpoint := url.URL{Scheme: v.scheme, Host: credentials.Endpoint, Path: "/"}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create aliyun captcha request: %w", err)
	}
	now := time.Now().UTC()
	if v.now != nil {
		now = v.now().UTC()
	}
	nonce := ""
	if v.nonce != nil {
		nonce, err = v.nonce()
		if err != nil {
			return nil, fmt.Errorf("create aliyun captcha request nonce: %w", err)
		}
	}
	signAliyunCaptchaRequest(req, []byte(body), credentials, now, nonce)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send aliyun captcha request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, aliyunCaptchaResponseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read aliyun captcha response: %w", err)
	}
	if len(payload) > aliyunCaptchaResponseLimit {
		return nil, fmt.Errorf("aliyun captcha response exceeds %d bytes", aliyunCaptchaResponseLimit)
	}
	var envelope aliyunCaptchaResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode aliyun captcha response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || (envelope.Code != "" && !strings.EqualFold(envelope.Code, "Success")) {
		return nil, &service.AliyunCaptchaAPIError{
			Code:      strings.TrimSpace(envelope.Code),
			Message:   strings.TrimSpace(envelope.Message),
			RequestID: strings.TrimSpace(envelope.RequestID),
		}
	}
	if envelope.Result == nil {
		return nil, fmt.Errorf("decode aliyun captcha response: missing Result")
	}
	return &service.AliyunCaptchaVerifyResult{
		VerifyResult: envelope.Result.VerifyResult,
		VerifyCode:   envelope.Result.VerifyCode,
		RequestID:    envelope.RequestID,
	}, nil
}

func signAliyunCaptchaRequest(req *http.Request, body []byte, credentials service.AliyunCaptchaCredentials, now time.Time, nonce string) {
	payloadHash := sha256Hex(body)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", aliyunCaptchaContentType)
	req.Header.Set("X-Acs-Action", aliyunCaptchaAction)
	req.Header.Set("X-Acs-Content-Sha256", payloadHash)
	req.Header.Set("X-Acs-Date", now.UTC().Format("2006-01-02T15:04:05Z"))
	req.Header.Set("X-Acs-Signature-Nonce", nonce)
	req.Header.Set("X-Acs-Version", aliyunCaptchaVersion)

	signedHeaders := "content-type;host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version"
	canonicalHeaders := strings.Join([]string{
		"content-type:" + aliyunCaptchaContentType,
		"host:" + strings.ToLower(req.URL.Host),
		"x-acs-action:" + aliyunCaptchaAction,
		"x-acs-content-sha256:" + payloadHash,
		"x-acs-date:" + req.Header.Get("X-Acs-Date"),
		"x-acs-signature-nonce:" + nonce,
		"x-acs-version:" + aliyunCaptchaVersion,
	}, "\n") + "\n"
	canonicalRequest := strings.Join([]string{
		req.Method,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	stringToSign := aliyunCaptchaAlgorithm + "\n" + sha256Hex([]byte(canonicalRequest))
	signature := hex.EncodeToString(hmacSHA256([]byte(credentials.AccessKeySecret), stringToSign))
	req.Header.Set("Authorization", aliyunCaptchaAlgorithm+" Credential="+credentials.AccessKeyID+",SignedHeaders="+signedHeaders+",Signature="+signature)
}

func newAliyunCaptchaNonce() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
