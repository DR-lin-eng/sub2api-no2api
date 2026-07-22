package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/httpclient"
)

type capVerifier struct {
	httpClient *http.Client
}

func NewCapVerifier() service.CapVerifier {
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &capVerifier{httpClient: sharedClient}
}

func (v *capVerifier) VerifyToken(ctx context.Context, apiEndpoint, secretKey, token string) (*service.CapVerifyResponse, error) {
	body, err := json.Marshal(map[string]string{"secret": secretKey, "response": token})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	verifyURL := strings.TrimRight(strings.TrimSpace(apiEndpoint), "/") + "/siteverify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var result service.CapVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
