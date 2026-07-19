package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

type recaptchaVerifier struct {
	httpClient *http.Client
	verifyURL  string
}

func NewRecaptchaVerifier() service.RecaptchaVerifier {
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &recaptchaVerifier{httpClient: sharedClient, verifyURL: recaptchaVerifyURL}
}

func (v *recaptchaVerifier) VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*service.RecaptchaVerifyResponse, error) {
	formData := url.Values{}
	formData.Set("secret", secretKey)
	formData.Set("response", token)
	if remoteIP != "" {
		formData.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var result service.RecaptchaVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
