package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

const openAIUsageBaseURL = "https://chatgpt.com/backend-api"

// These read-only usage resources use the same credentials, proxy, IPv6 route
// and TLS policy as quota queries. Never accept an upstream URL from the UI.
func (s *OpenAIQuotaService) usageHTTP(ctx context.Context, accountID int64, method, path string, query url.Values, body any) (int, http.Header, []byte, error) {
	var route platformegress.Route
	token, principal, proxyURL, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID, &route)
	if err != nil {
		return 0, nil, nil, err
	}
	headers, _, err := s.buildCodexQuotaHeaders(ctx, accountID, token, principal, fedRAMP)
	if err != nil {
		return 0, nil, nil, err
	}
	headers["OAI-App-Brand"] = "codex"
	headers["x-openai-codex-pricing-chooser"] = "1"
	headers["x-openai-codex-usage-categories"] = "1"
	if strings.Contains(path, "monthly-usage") {
		headers["Cache-Control"], headers["Pragma"] = "no-store", "no-cache"
	}
	if path == "/codex/notification/subscriptions" {
		headers["X-OpenAI-Codex-Initial-Snapshot"] = "usage"
	}
	target := openAIUsageBaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
	}
	if cLevelTransportSimulationEnabled(s.settingService) && s.httpUpstream != nil {
		account, err := s.quotaCredentialAccount(ctx, accountID)
		if err != nil {
			return 0, nil, nil, err
		}
		return s.doCodexQuotaHTTP(ctx, account, method, target, headers, payload)
	}
	callCtx, cancel := context.WithTimeout(withEgressRouteContext(ctx, route, s.cfg), openaiQuotaUpstreamTimeout)
	defer cancel()
	client, err := s.privacyClientFactory(callCtx, proxyURL)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("usage transport unavailable")
	}
	request := client.R().SetContext(callCtx).SetHeaders(headers)
	if body != nil {
		request.SetBodyJsonBytes(payload)
	}
	resp, err := request.Send(method, target)
	if err != nil {
		// Discovery replies and WS errors can contain signed URLs. Do not expose
		// raw transport errors, upstream response bodies or URLs to the browser.
		return 0, nil, nil, fmt.Errorf("usage request failed")
	}
	if int64(len(resp.Bytes())) > openAICodexAuxiliaryBodyLimit {
		return 0, nil, nil, fmt.Errorf("usage response exceeds the size limit")
	}
	return resp.StatusCode, resp.Header.Clone(), resp.Bytes(), nil
}
