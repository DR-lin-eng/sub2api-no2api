package repository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

type cloudflareWAFFakeAPI struct {
	mu           sync.Mutex
	rulesetID    string
	rules        map[string]cloudflareWAFRule
	getCalls     int
	patchCalls   int
	dryRunCalls  int
	graphqlCalls int
	lastQuery    string
}

func newCloudflareWAFFakeAPI(ruleIDs ...string) *cloudflareWAFFakeAPI {
	rules := make(map[string]cloudflareWAFRule, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		rules[ruleID] = cloudflareWAFRule{
			ID: ruleID, Action: "block", Enabled: true, Expression: "false",
		}
	}
	return &cloudflareWAFFakeAPI{rulesetID: strings.Repeat("f", 32), rules: rules}
}

func (f *cloudflareWAFFakeAPI) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	entrypointPath := "/zones/" + strings.Repeat("a", 32) + "/rulesets/phases/" + cloudflareWAFPhase + "/entrypoint"
	if request.Method == http.MethodGet && request.URL.Path == entrypointPath {
		f.getCalls++
		f.writeRuleset(w)
		return
	}
	patchPrefix := "/zones/" + strings.Repeat("a", 32) + "/rulesets/" + f.rulesetID + "/rules/"
	if request.Method == http.MethodPatch && strings.HasPrefix(request.URL.Path, patchPrefix) {
		ruleID := strings.TrimPrefix(request.URL.Path, patchPrefix)
		rule, ok := f.rules[ruleID]
		if !ok {
			http.NotFound(w, request)
			return
		}
		var payload struct {
			Expression string `json:"expression"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.URL.Query().Get("dry_run") == "true" {
			f.dryRunCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": nil})
			return
		}
		rule.Expression = payload.Expression
		f.rules[ruleID] = rule
		f.patchCalls++
		f.writeRuleset(w)
		return
	}
	if request.Method == http.MethodPost && request.URL.Path == "/graphql" {
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.graphqlCalls++
		f.lastQuery = payload["query"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"viewer": map[string]any{"zones": []any{map[string]any{
				"hostnameRequests": []any{
					map[string]any{"dimensions": map[string]any{"clientRequestHTTPHost": "api.example.com"}, "sum": map[string]any{"requests": 1000}},
					map[string]any{"dimensions": map[string]any{"clientRequestHTTPHost": "edge.example.com"}, "sum": map[string]any{"requests": 234}},
				},
				"wafBlocks": []any{
					map[string]any{"dimensions": map[string]any{"clientRequestHTTPHost": "api.example.com"}, "count": 12},
					map[string]any{"dimensions": map[string]any{"clientRequestHTTPHost": "edge.example.com"}, "count": 3},
				},
			}}}},
		})
		return
	}
	http.NotFound(w, request)
}

func (f *cloudflareWAFFakeAPI) writeRuleset(w http.ResponseWriter) {
	rules := make([]cloudflareWAFRule, 0, len(f.rules))
	for _, rule := range f.rules {
		rules = append(rules, rule)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"errors":  []any{},
		"result": cloudflareWAFRuleset{
			ID: f.rulesetID, Kind: "zone", Name: "Custom Rules", Phase: cloudflareWAFPhase, Rules: rules,
		},
	})
}

func cloudflareWAFSettingsForTest(ruleIDs ...string) service.CloudflareIngressSettings {
	return service.CloudflareIngressSettings{
		Enabled: true, Mode: service.CloudflareIngressModeWAFCustomRules,
		ZoneID: strings.Repeat("a", 32), APIToken: "secret-token",
		WAFHostname: "api.example.com", WAFHostnames: []string{"api.example.com"}, WAFRuleIDs: ruleIDs,
		WAFSyncIntervalSeconds: 5, AnalyticsIntervalSeconds: 60,
		RequestTimeoutSeconds: 1, QueueCapacity: 16, MaxActiveRules: 1000,
		ReconcileIntervalSeconds: 300,
	}
}

func TestCloudflareWAFExpressionsShardWithinLimit(t *testing.T) {
	entries := make([]cloudflareWAFStateEntry, 0, 700)
	for index := 0; index < 700; index++ {
		entries = append(entries, cloudflareWAFStateEntry{Value: fmt.Sprintf("2001:db8:%x::/64", index)})
	}
	hostnames := []string{"api.example.com", "edge.example.com"}
	expressions, included, overflow := cloudflareWAFExpressions(hostnames, entries, 2)
	require.Len(t, expressions, 2)
	require.Equal(t, len(entries), included+overflow)
	require.Positive(t, overflow)
	for _, expression := range expressions {
		require.LessOrEqual(t, len(expression), cloudflareWAFExpressionMaxLength)
		require.Contains(t, expression, `http.host in {"api.example.com" "edge.example.com"}`)
	}

	empty, included, overflow := cloudflareWAFExpressions(hostnames, nil, 2)
	require.Equal(t, []string{
		`(http.host in {"api.example.com" "edge.example.com"} and ip.src in {0.0.0.0})`,
		`(http.host in {"api.example.com" "edge.example.com"} and ip.src in {0.0.0.0})`,
	}, empty)
	require.Zero(t, included)
	require.Zero(t, overflow)
}

func TestCloudflareWAFExpressionsKeepUnrelatedShardsStable(t *testing.T) {
	entries := make([]cloudflareWAFStateEntry, 0, 100)
	for index := 1; index <= 100; index++ {
		entries = append(entries, cloudflareWAFStateEntry{Value: fmt.Sprintf("198.51.100.%d", index)})
	}
	before, included, overflow := cloudflareWAFExpressions([]string{"api.example.com"}, entries, 4)
	require.Equal(t, len(entries), included)
	require.Zero(t, overflow)

	entries = append(entries, cloudflareWAFStateEntry{Value: "203.0.113.200"})
	after, included, overflow := cloudflareWAFExpressions([]string{"api.example.com"}, entries, 4)
	require.Equal(t, len(entries), included)
	require.Zero(t, overflow)
	changed := 0
	for index := range before {
		if before[index] != after[index] {
			changed++
		}
	}
	require.Equal(t, 1, changed)
}

func TestCloudflareWAFClientSyncsRulesAndQueriesAnalytics(t *testing.T) {
	ruleIDs := []string{strings.Repeat("b", 32), strings.Repeat("c", 32)}
	fake := newCloudflareWAFFakeAPI(ruleIDs...)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := cloudflareWAFSettingsForTest(ruleIDs...)
	client := newCloudflareWAFClient(server.URL, settings)

	require.NoError(t, client.validateRules(t.Context(), ruleIDs))
	expressions := []string{
		`(http.host eq "api.example.com" and ip.src in {192.0.2.1})`,
		`(http.host eq "api.example.com" and ip.src in {0.0.0.0})`,
	}
	changed, err := client.syncExpressions(t.Context(), ruleIDs, expressions)
	require.NoError(t, err)
	require.Equal(t, 2, changed)
	changed, err = client.syncExpressions(t.Context(), ruleIDs, expressions)
	require.NoError(t, err)
	require.Zero(t, changed)

	hostnames := []string{"api.example.com", "edge.example.com"}
	analytics, err := client.queryAnalytics(t.Context(), hostnames, ruleIDs, time.Now())
	require.NoError(t, err)
	require.Equal(t, uint64(1234), analytics.HostnameRequests)
	require.Equal(t, uint64(15), analytics.BlockedRequests)
	require.Equal(t, []cloudflareWAFHostnameAnalytics{
		{Hostname: "api.example.com", Requests: 1000, BlockedRequests: 12},
		{Hostname: "edge.example.com", Requests: 234, BlockedRequests: 3},
	}, analytics.Hostnames)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, 2, fake.patchCalls)
	require.Equal(t, 1, fake.dryRunCalls)
	require.Equal(t, 1, fake.graphqlCalls)
	require.Contains(t, fake.lastQuery, `clientRequestHTTPHost_in: ["api.example.com", "edge.example.com"]`)
	require.Contains(t, fake.lastQuery, `action: "block"`)
	for _, ruleID := range ruleIDs {
		require.Contains(t, fake.lastQuery, ruleID)
	}
}

func TestCloudflareWAFSyncSkipsCloudflareWhenDesiredRevisionIsUnchanged(t *testing.T) {
	ruleID := strings.Repeat("b", 32)
	fake := newCloudflareWAFFakeAPI(ruleID)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := cloudflareWAFSettingsForTest(ruleID)
	store := newCloudflareWAFStateStoreForTest(t)
	blocker := newCloudflareIngressBlocker(settings, nil)
	blocker.wafState = store
	blocker.wafClientBuild = func(next service.CloudflareIngressSettings) *cloudflareWAFClient {
		return newCloudflareWAFClient(server.URL, next)
	}
	runtime := blocker.buildRuntime(settings)
	t.Cleanup(blocker.Stop)
	require.NoError(t, store.UpsertBlocks(t.Context(), []cloudflareBlockRequest{{
		value: "203.0.113.10", expiresAt: time.Now().Add(time.Minute),
	}}))

	require.NoError(t, blocker.syncWAF(runtime, false))
	fake.mu.Lock()
	firstGetCalls := fake.getCalls
	firstPatchCalls := fake.patchCalls
	fake.mu.Unlock()
	require.Equal(t, 1, firstGetCalls)
	require.Equal(t, 1, firstPatchCalls)

	require.NoError(t, blocker.syncWAF(runtime, false))
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, firstGetCalls, fake.getCalls)
	require.Equal(t, firstPatchCalls, fake.patchCalls)
}

func TestCloudflareWAFWorkerPersistsQueuedBlockBeforeBatchSync(t *testing.T) {
	ruleID := strings.Repeat("b", 32)
	fake := newCloudflareWAFFakeAPI(ruleID)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := cloudflareWAFSettingsForTest(ruleID)
	store := newCloudflareWAFStateStoreForTest(t)
	blocker := newCloudflareIngressBlocker(settings, nil)
	blocker.wafState = store
	blocker.wafClientBuild = func(next service.CloudflareIngressSettings) *cloudflareWAFClient {
		return newCloudflareWAFClient(server.URL, next)
	}
	blocker.initial = blocker.buildRuntime(settings)
	blocker.pollInterval = 10 * time.Millisecond
	blocker.Start()
	t.Cleanup(blocker.Stop)
	require.Eventually(t, func() bool { return blocker.Health().Running }, 2*time.Second, 10*time.Millisecond)

	require.True(t, blocker.EnqueueBlock("203.0.113.42", time.Now().Add(time.Minute)))
	require.Eventually(t, func() bool {
		count, err := store.CountActive(t.Context(), time.Now())
		return err == nil && count == 1 && blocker.Health().QueueDepth == 0
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, blocker.syncWAF(blocker.initial, false))
	fake.mu.Lock()
	expression := fake.rules[ruleID].Expression
	fake.mu.Unlock()
	require.Contains(t, expression, `http.host eq "api.example.com"`)
	require.Contains(t, expression, "203.0.113.42")
	require.Equal(t, 1, blocker.Health().ActiveRules)
	require.NotNil(t, blocker.Health().WAF)
	require.Equal(t, 1, blocker.Health().WAF.SyncedEntries)
}
