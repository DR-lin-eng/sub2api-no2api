package repository

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

const (
	cloudflareWAFPhase               = "http_request_firewall_custom"
	cloudflareWAFExpressionMaxLength = 4096
	cloudflareWAFAnalyticsWindow     = 24 * time.Hour
)

type cloudflareWAFClient struct {
	api       *cloudflareIngressClient
	zoneID    string
	hostnames []string
}

type cloudflareWAFRule struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Expression  string `json:"expression"`
	Ref         string `json:"ref"`
}

type cloudflareWAFRuleset struct {
	ID    string              `json:"id"`
	Kind  string              `json:"kind"`
	Name  string              `json:"name"`
	Phase string              `json:"phase"`
	Rules []cloudflareWAFRule `json:"rules"`
}

type cloudflareWAFRulesetResponse struct {
	Success bool                      `json:"success"`
	Errors  []cloudflareResponseError `json:"errors"`
	Result  cloudflareWAFRuleset      `json:"result"`
}

type cloudflareWAFAnalytics struct {
	HostnameRequests uint64
	BlockedRequests  uint64
	Hostnames        []cloudflareWAFHostnameAnalytics
	WindowStart      time.Time
	WindowEnd        time.Time
}

type cloudflareWAFHostnameAnalytics struct {
	Hostname        string
	Requests        uint64
	BlockedRequests uint64
}

type cloudflareGraphQLResponse struct {
	Data struct {
		Viewer struct {
			Zones []struct {
				HostnameRequests []struct {
					Dimensions struct {
						Hostname string `json:"clientRequestHTTPHost"`
					} `json:"dimensions"`
					Sum struct {
						Requests uint64 `json:"requests"`
					} `json:"sum"`
				} `json:"hostnameRequests"`
				WAFBlocks []struct {
					Count      uint64 `json:"count"`
					Dimensions struct {
						Hostname string `json:"clientRequestHTTPHost"`
					} `json:"dimensions"`
				} `json:"wafBlocks"`
			} `json:"zones"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func newCloudflareWAFClient(
	baseURL string,
	settings service.CloudflareIngressSettings,
) *cloudflareWAFClient {
	if strings.TrimSpace(settings.ZoneID) == "" || strings.TrimSpace(settings.APIToken) == "" {
		return nil
	}
	hostnames := append([]string(nil), settings.WAFHostnames...)
	if len(hostnames) == 0 && settings.WAFHostname != "" {
		hostnames = []string{settings.WAFHostname}
	}
	timeout := time.Duration(settings.RequestTimeoutSeconds) * time.Second
	return &cloudflareWAFClient{
		api:       newCloudflareIngressClient(baseURL, settings.ZoneID, settings.APIToken, timeout),
		zoneID:    settings.ZoneID,
		hostnames: hostnames,
	}
}

func (c *cloudflareWAFClient) validateRules(ctx context.Context, ruleIDs []string) error {
	if c == nil || c.api == nil {
		return errors.New("cloudflare WAF client is unavailable")
	}
	ruleset, err := c.getCustomRuleset(ctx)
	if err != nil {
		return err
	}
	rules, err := selectCloudflareWAFRules(ruleset, ruleIDs)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return errors.New("at least one cloudflare WAF rule is required")
	}
	emptyExpressions, _, _ := cloudflareWAFExpressions(c.hostnames, nil, 1)
	return c.patchRuleExpression(ctx, ruleset.ID, rules[0].ID, emptyExpressions[0], true)
}

func (c *cloudflareWAFClient) syncExpressions(
	ctx context.Context,
	ruleIDs []string,
	expressions []string,
) (int, error) {
	if len(ruleIDs) != len(expressions) {
		return 0, errors.New("cloudflare WAF rule and expression counts do not match")
	}
	ruleset, err := c.getCustomRuleset(ctx)
	if err != nil {
		return 0, err
	}
	rules, err := selectCloudflareWAFRules(ruleset, ruleIDs)
	if err != nil {
		return 0, err
	}
	changed := 0
	for index, rule := range rules {
		if rule.Expression == expressions[index] {
			continue
		}
		if err := c.patchRuleExpression(ctx, ruleset.ID, rule.ID, expressions[index], false); err != nil {
			return changed, fmt.Errorf("update cloudflare WAF rule %s: %w", rule.ID, err)
		}
		changed++
	}
	return changed, nil
}

func (c *cloudflareWAFClient) queryAnalytics(
	ctx context.Context,
	hostnames []string,
	ruleIDs []string,
	now time.Time,
) (cloudflareWAFAnalytics, error) {
	if c == nil || c.api == nil {
		return cloudflareWAFAnalytics{}, errors.New("cloudflare WAF client is unavailable")
	}
	windowEnd := now.UTC()
	windowStart := windowEnd.Add(-cloudflareWAFAnalyticsWindow)
	quotedRuleIDs := make([]string, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		quotedRuleIDs = append(quotedRuleIDs, strconv.Quote(ruleID))
	}
	quotedHostnames := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		quotedHostnames = append(quotedHostnames, strconv.Quote(hostname))
	}
	hostnameFilter := strings.Join(quotedHostnames, ", ")
	query := fmt.Sprintf(`query {
  viewer {
    zones(filter: {zoneTag: %s}) {
      hostnameRequests: httpRequestsAdaptiveGroups(
        limit: 100
        filter: {datetime_geq: %s, datetime_lt: %s, clientRequestHTTPHost_in: [%s]}
      ) { dimensions { clientRequestHTTPHost } sum { requests } }
      wafBlocks: firewallEventsAdaptiveGroups(
        limit: 100
        filter: {datetime_geq: %s, datetime_lt: %s, action: "block", clientRequestHTTPHost_in: [%s], ruleId_in: [%s]}
      ) { count dimensions { clientRequestHTTPHost } }
    }
  }
}`,
		strconv.Quote(c.zoneID),
		strconv.Quote(windowStart.Format(time.RFC3339)),
		strconv.Quote(windowEnd.Format(time.RFC3339)),
		hostnameFilter,
		strconv.Quote(windowStart.Format(time.RFC3339)),
		strconv.Quote(windowEnd.Format(time.RFC3339)),
		hostnameFilter,
		strings.Join(quotedRuleIDs, ", "),
	)
	var response cloudflareGraphQLResponse
	if err := c.api.do(ctx, http.MethodPost, "/graphql", nil, map[string]string{"query": query}, &response); err != nil {
		return cloudflareWAFAnalytics{}, fmt.Errorf("query Cloudflare WAF analytics: %w", err)
	}
	if len(response.Errors) > 0 {
		messages := make([]string, 0, len(response.Errors))
		for _, item := range response.Errors {
			if message := strings.TrimSpace(item.Message); message != "" {
				messages = append(messages, message)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "unknown GraphQL error")
		}
		return cloudflareWAFAnalytics{}, fmt.Errorf("query Cloudflare WAF analytics: %s", strings.Join(messages, "; "))
	}
	if len(response.Data.Viewer.Zones) != 1 {
		return cloudflareWAFAnalytics{}, errors.New("cloudflare WAF analytics returned no matching zone")
	}
	zone := response.Data.Viewer.Zones[0]
	result := cloudflareWAFAnalytics{
		Hostnames:   make([]cloudflareWAFHostnameAnalytics, 0, len(hostnames)),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}
	byHostname := make(map[string]int, len(hostnames))
	for _, hostname := range hostnames {
		result.Hostnames = append(result.Hostnames, cloudflareWAFHostnameAnalytics{Hostname: hostname})
		byHostname[hostname] = len(result.Hostnames) - 1
	}
	for _, group := range zone.HostnameRequests {
		result.HostnameRequests += group.Sum.Requests
		if index, ok := byHostname[strings.ToLower(group.Dimensions.Hostname)]; ok {
			result.Hostnames[index].Requests += group.Sum.Requests
		}
	}
	for _, group := range zone.WAFBlocks {
		result.BlockedRequests += group.Count
		if index, ok := byHostname[strings.ToLower(group.Dimensions.Hostname)]; ok {
			result.Hostnames[index].BlockedRequests += group.Count
		}
	}
	return result, nil
}

func (c *cloudflareWAFClient) getCustomRuleset(ctx context.Context) (cloudflareWAFRuleset, error) {
	path := "/zones/" + url.PathEscape(c.zoneID) + "/rulesets/phases/" + cloudflareWAFPhase + "/entrypoint"
	var response cloudflareWAFRulesetResponse
	if err := c.api.do(ctx, http.MethodGet, path, nil, nil, &response); err != nil {
		return cloudflareWAFRuleset{}, fmt.Errorf("get Cloudflare custom WAF ruleset: %w", err)
	}
	if !response.Success {
		return cloudflareWAFRuleset{}, cloudflareErrors(response.Errors)
	}
	if response.Result.ID == "" || response.Result.Phase != cloudflareWAFPhase {
		return cloudflareWAFRuleset{}, errors.New("cloudflare custom WAF entrypoint ruleset is unavailable")
	}
	return response.Result, nil
}

func (c *cloudflareWAFClient) patchRuleExpression(
	ctx context.Context,
	rulesetID string,
	ruleID string,
	expression string,
	dryRun bool,
) error {
	path := "/zones/" + url.PathEscape(c.zoneID) + "/rulesets/" + url.PathEscape(rulesetID) + "/rules/" + url.PathEscape(ruleID)
	query := url.Values(nil)
	if dryRun {
		query = url.Values{"dry_run": []string{"true"}}
	}
	var response cloudflareWAFRulesetResponse
	if err := c.api.do(ctx, http.MethodPatch, path, query, map[string]string{"expression": expression}, &response); err != nil {
		return err
	}
	if !response.Success {
		return cloudflareErrors(response.Errors)
	}
	return nil
}

func selectCloudflareWAFRules(ruleset cloudflareWAFRuleset, ruleIDs []string) ([]cloudflareWAFRule, error) {
	available := make(map[string]cloudflareWAFRule, len(ruleset.Rules))
	for _, rule := range ruleset.Rules {
		available[strings.ToLower(rule.ID)] = rule
	}
	selected := make([]cloudflareWAFRule, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		rule, ok := available[strings.ToLower(ruleID)]
		if !ok {
			return nil, fmt.Errorf("cloudflare WAF rule %s was not found in the zone custom ruleset", ruleID)
		}
		if !strings.EqualFold(rule.Action, "block") {
			return nil, fmt.Errorf("cloudflare WAF rule %s must use the block action", ruleID)
		}
		if !rule.Enabled {
			return nil, fmt.Errorf("cloudflare WAF rule %s must be enabled", ruleID)
		}
		selected = append(selected, rule)
	}
	return selected, nil
}

func cloudflareWAFExpressions(hostnames []string, entries []cloudflareWAFStateEntry, ruleCount int) ([]string, int, int) {
	if ruleCount < 1 {
		return nil, 0, len(entries)
	}
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		if value := strings.TrimSpace(entry.Value); value != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	hostPredicate := cloudflareWAFHostnamePredicate(hostnames)
	prefix := "(" + hostPredicate + " and ip.src in {"
	suffix := "})"
	empty := "(" + hostPredicate + " and ip.src in {0.0.0.0})"
	expressions := make([]string, ruleCount)
	shards := make([][]string, ruleCount)
	lengths := make([]int, ruleCount)
	for index := range lengths {
		lengths[index] = len(prefix) + len(suffix)
	}
	included := 0
	for _, value := range values {
		preferred := cloudflareWAFShard(value, ruleCount)
		for offset := 0; offset < ruleCount; offset++ {
			shard := (preferred + offset) % ruleCount
			addition := len(value)
			if len(shards[shard]) > 0 {
				addition++
			}
			if lengths[shard]+addition > cloudflareWAFExpressionMaxLength {
				continue
			}
			shards[shard] = append(shards[shard], value)
			lengths[shard] += addition
			included++
			break
		}
	}
	for shard := range ruleCount {
		if len(shards[shard]) == 0 {
			expressions[shard] = empty
		} else {
			expressions[shard] = prefix + strings.Join(shards[shard], " ") + suffix
		}
	}
	return expressions, included, len(values) - included
}

func cloudflareWAFHostnamePredicate(hostnames []string) string {
	if len(hostnames) == 0 {
		return `http.host eq ""`
	}
	if len(hostnames) == 1 {
		return "http.host eq " + strconv.Quote(hostnames[0])
	}
	quoted := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		quoted = append(quoted, strconv.Quote(hostname))
	}
	return "http.host in {" + strings.Join(quoted, " ") + "}"
}

func cloudflareWAFShard(value string, ruleCount int) int {
	if ruleCount <= 1 {
		return 0
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(value))
	return int(hasher.Sum32() % uint32(ruleCount))
}
