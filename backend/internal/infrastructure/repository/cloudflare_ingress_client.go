package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	cloudflareAPIBaseURL       = "https://api.cloudflare.com/client/v4"
	cloudflareManagedNote      = "sub2api-invalid-auth"
	cloudflareMaxResponseBytes = 1 << 20
	cloudflareRulesPageSize    = 100
	cloudflareMaxRulePages     = 500
)

var errCloudflareRuleNotFound = errors.New("cloudflare access rule not found")

type cloudflareIngressClient struct {
	baseURL string
	zoneID  string
	token   string
	client  *http.Client
}

type cloudflareAccessRuleConfiguration struct {
	Target string `json:"target"`
	Value  string `json:"value"`
}

type cloudflareAccessRule struct {
	ID            string                            `json:"id"`
	Mode          string                            `json:"mode"`
	Configuration cloudflareAccessRuleConfiguration `json:"configuration"`
	Notes         string                            `json:"notes"`
}

type cloudflareResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareRuleResponse struct {
	Success bool                      `json:"success"`
	Errors  []cloudflareResponseError `json:"errors"`
	Result  cloudflareAccessRule      `json:"result"`
}

type cloudflareRuleListResponse struct {
	Success bool                      `json:"success"`
	Errors  []cloudflareResponseError `json:"errors"`
	Result  []cloudflareAccessRule    `json:"result"`
	Info    struct {
		Count      int `json:"count"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

type cloudflareRuleMutation struct {
	Mode          string                            `json:"mode"`
	Configuration cloudflareAccessRuleConfiguration `json:"configuration"`
	Notes         string                            `json:"notes"`
}

func newCloudflareIngressClient(baseURL, zoneID, token string, timeout time.Duration) *cloudflareIngressClient {
	return &cloudflareIngressClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		zoneID:  strings.TrimSpace(zoneID),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *cloudflareIngressClient) validateAccess(ctx context.Context) error {
	_, err := c.listManagedRules(ctx, "")
	return err
}

func (c *cloudflareIngressClient) createRule(ctx context.Context, target, value string, expiresAt time.Time) (cloudflareAccessRule, error) {
	payload := cloudflareRuleMutation{
		Mode:          "block",
		Configuration: cloudflareAccessRuleConfiguration{Target: target, Value: value},
		Notes:         cloudflareRuleNote(expiresAt),
	}
	var response cloudflareRuleResponse
	if err := c.do(ctx, http.MethodPost, c.rulesPath(), nil, payload, &response); err != nil {
		return cloudflareAccessRule{}, err
	}
	if strings.TrimSpace(response.Result.ID) == "" {
		return cloudflareAccessRule{}, errors.New("cloudflare create access rule returned no rule id")
	}
	return response.Result, nil
}

func (c *cloudflareIngressClient) getRule(ctx context.Context, ruleID string) (cloudflareAccessRule, error) {
	var response cloudflareRuleResponse
	if err := c.do(ctx, http.MethodGet, c.rulePath(ruleID), nil, nil, &response); err != nil {
		return cloudflareAccessRule{}, err
	}
	return response.Result, nil
}

func (c *cloudflareIngressClient) updateRule(ctx context.Context, rule cloudflareAccessRule, expiresAt time.Time) error {
	payload := cloudflareRuleMutation{
		Mode:          "block",
		Configuration: rule.Configuration,
		Notes:         cloudflareRuleNote(expiresAt),
	}
	var response cloudflareRuleResponse
	return c.do(ctx, http.MethodPatch, c.rulePath(rule.ID), nil, payload, &response)
}

func (c *cloudflareIngressClient) deleteRule(ctx context.Context, ruleID string) error {
	var response cloudflareRuleResponse
	err := c.do(ctx, http.MethodDelete, c.rulePath(ruleID), nil, nil, &response)
	if errors.Is(err, errCloudflareRuleNotFound) {
		return nil
	}
	return err
}

func (c *cloudflareIngressClient) listManagedRules(ctx context.Context, configurationValue string) ([]cloudflareAccessRule, error) {
	result := make([]cloudflareAccessRule, 0)
	for page := 1; page <= cloudflareMaxRulePages; page++ {
		query := url.Values{
			"mode":     []string{"block"},
			"notes":    []string{cloudflareManagedNote},
			"match":    []string{"all"},
			"page":     []string{strconv.Itoa(page)},
			"per_page": []string{strconv.Itoa(cloudflareRulesPageSize)},
		}
		if configurationValue != "" {
			query.Set("configuration.value", configurationValue)
		}
		var response cloudflareRuleListResponse
		if err := c.do(ctx, http.MethodGet, c.rulesPath(), query, nil, &response); err != nil {
			return nil, err
		}
		result = append(result, response.Result...)
		if response.Info.TotalCount > 0 {
			if response.Info.TotalCount <= len(result) {
				return result, nil
			}
			if len(response.Result) == 0 {
				return nil, errors.New("cloudflare access rule pagination ended before total_count")
			}
			continue
		}
		pageSize := response.Info.PerPage
		if pageSize <= 0 {
			pageSize = cloudflareRulesPageSize
		}
		if len(response.Result) < pageSize {
			return result, nil
		}
	}
	return nil, fmt.Errorf("cloudflare access rule pagination exceeds %d pages", cloudflareMaxRulePages)
}

func (c *cloudflareIngressClient) do(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	payload any,
	destination any,
) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode cloudflare request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("build cloudflare request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	limited := io.LimitReader(response.Body, cloudflareMaxResponseBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read cloudflare response: %w", err)
	}
	if len(encoded) > cloudflareMaxResponseBytes {
		return errors.New("cloudflare response exceeds 1 MiB")
	}
	if response.StatusCode == http.StatusNotFound {
		return errCloudflareRuleNotFound
	}
	if destination != nil && len(encoded) > 0 {
		if err := json.Unmarshal(encoded, destination); err != nil {
			return fmt.Errorf("decode cloudflare response (status %d): %w", response.StatusCode, err)
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return cloudflareAPIError(response.StatusCode, encoded)
	}
	switch value := destination.(type) {
	case *cloudflareRuleResponse:
		if !value.Success {
			return cloudflareErrors(value.Errors)
		}
	case *cloudflareRuleListResponse:
		if !value.Success {
			return cloudflareErrors(value.Errors)
		}
	}
	return nil
}

func (c *cloudflareIngressClient) rulesPath() string {
	return "/zones/" + url.PathEscape(c.zoneID) + "/firewall/access_rules/rules"
}

func (c *cloudflareIngressClient) rulePath(ruleID string) string {
	return c.rulesPath() + "/" + url.PathEscape(strings.TrimSpace(ruleID))
}

func cloudflareAPIError(status int, encoded []byte) error {
	var envelope struct {
		Errors []cloudflareResponseError `json:"errors"`
	}
	if json.Unmarshal(encoded, &envelope) == nil && len(envelope.Errors) > 0 {
		return fmt.Errorf("cloudflare API status %d: %w", status, cloudflareErrors(envelope.Errors))
	}
	return fmt.Errorf("cloudflare API status %d", status)
}

func cloudflareErrors(items []cloudflareResponseError) error {
	if len(items) == 0 {
		return errors.New("cloudflare API reported failure")
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = "unknown error"
		}
		if item.Code != 0 {
			message = fmt.Sprintf("%d: %s", item.Code, message)
		}
		parts = append(parts, message)
	}
	return errors.New(strings.Join(parts, "; "))
}

func cloudflareRuleNote(expiresAt time.Time) string {
	return cloudflareManagedNote + ";expires=" + expiresAt.UTC().Format(time.RFC3339Nano)
}

func cloudflareRuleExpiry(notes string) (time.Time, bool) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(notes)), cloudflareManagedNote+";") {
		return time.Time{}, false
	}
	for _, part := range strings.Split(notes, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "expires") {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		return expiresAt, err == nil
	}
	return time.Time{}, false
}
