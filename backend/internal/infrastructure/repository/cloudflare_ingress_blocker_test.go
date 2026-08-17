package repository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

type cloudflareFakeAPI struct {
	mu            sync.Mutex
	rules         map[string]cloudflareAccessRule
	nextID        int
	createFailure int
	createCalls   int
	patchCalls    int
	deleteCalls   int
	lastAuth      string
}

func newCloudflareFakeAPI() *cloudflareFakeAPI {
	return &cloudflareFakeAPI{rules: make(map[string]cloudflareAccessRule)}
}

func (f *cloudflareFakeAPI) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastAuth = request.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/json")
	basePath := "/zones/test-zone/firewall/access_rules/rules"
	if !strings.HasPrefix(request.URL.Path, basePath) {
		http.NotFound(w, request)
		return
	}
	ruleID := ""
	if strings.HasPrefix(request.URL.Path, basePath+"/") {
		ruleID = strings.TrimPrefix(request.URL.Path, basePath+"/")
	}
	switch request.Method {
	case http.MethodGet:
		if ruleID != "" {
			rule, ok := f.rules[ruleID]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": rule})
			return
		}
		items := make([]cloudflareAccessRule, 0, len(f.rules))
		for _, rule := range f.rules {
			if value := request.URL.Query().Get("configuration.value"); value != "" && rule.Configuration.Value != value {
				continue
			}
			if notes := request.URL.Query().Get("notes"); notes != "" && !strings.Contains(strings.ToLower(rule.Notes), strings.ToLower(notes)) {
				continue
			}
			items = append(items, rule)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"errors":  []any{},
			"result":  items,
			"result_info": map[string]any{
				"count": len(items), "page": 1, "per_page": cloudflareRulesPageSize, "total_count": len(items),
			},
		})
	case http.MethodPost:
		f.createCalls++
		if f.createFailure > 0 {
			f.createFailure--
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false, "errors": []map[string]any{{"code": 1000, "message": "temporary failure"}},
			})
			return
		}
		var payload cloudflareRuleMutation
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.nextID++
		rule := cloudflareAccessRule{
			ID: fmt.Sprintf("rule-%d", f.nextID), Mode: payload.Mode,
			Configuration: payload.Configuration, Notes: payload.Notes,
		}
		f.rules[rule.ID] = rule
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": rule})
	case http.MethodPatch:
		f.patchCalls++
		rule, ok := f.rules[ruleID]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		var payload cloudflareRuleMutation
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rule.Mode = payload.Mode
		rule.Configuration = payload.Configuration
		rule.Notes = payload.Notes
		f.rules[ruleID] = rule
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": rule})
	case http.MethodDelete:
		f.deleteCalls++
		if _, ok := f.rules[ruleID]; !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		delete(f.rules, ruleID)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": map[string]any{"id": ruleID}})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func cloudflareBlockerForTest(server *httptest.Server, fake *cloudflareFakeAPI) *cloudflareIngressBlocker {
	settings := service.CloudflareIngressSettings{
		Enabled: true, QueueCapacity: 16, MaxActiveRules: 8,
		ReconcileIntervalSeconds: 3600, RequestTimeoutSeconds: 1,
	}
	client := newCloudflareIngressClient(server.URL, "test-zone", "secret-token", time.Second)
	blocker := newCloudflareIngressBlocker(settings, client)
	blocker.pollInterval = 10 * time.Millisecond
	return blocker
}

func TestCloudflareIngressBlockerAppliesExtendsAndReleases(t *testing.T) {
	fake := newCloudflareFakeAPI()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	blocker := cloudflareBlockerForTest(server, fake)
	blocker.Start()
	t.Cleanup(blocker.Stop)

	firstExpiry := time.Now().Add(150 * time.Millisecond)
	require.True(t, blocker.EnqueueBlock("203.0.113.10", firstExpiry))
	require.Eventually(t, func() bool {
		return blocker.Health().ActiveRules == 1 && blocker.Health().Applied == 1
	}, 2*time.Second, 10*time.Millisecond)

	secondExpiry := time.Now().Add(350 * time.Millisecond)
	require.True(t, blocker.EnqueueBlock("203.0.113.10", secondExpiry))
	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return fake.patchCalls == 1
	}, 2*time.Second, 10*time.Millisecond)

	time.Sleep(time.Until(firstExpiry) + 30*time.Millisecond)
	require.Equal(t, 1, blocker.Health().ActiveRules, "the extended rule must survive its original expiry")
	require.Eventually(t, func() bool {
		return blocker.Health().ActiveRules == 0 && blocker.Health().Released == 1
	}, 2*time.Second, 10*time.Millisecond)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, 1, fake.createCalls)
	require.Equal(t, 1, fake.patchCalls)
	require.Equal(t, 1, fake.deleteCalls)
	require.Equal(t, "Bearer secret-token", fake.lastAuth)
}

func TestCloudflareIngressBlockerRetriesAPIFailureWithoutBlockingCaller(t *testing.T) {
	fake := newCloudflareFakeAPI()
	fake.createFailure = 1
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	blocker := cloudflareBlockerForTest(server, fake)
	blocker.Start()
	t.Cleanup(blocker.Stop)

	started := time.Now()
	require.True(t, blocker.EnqueueBlock("198.51.100.20", time.Now().Add(3*time.Second)))
	require.Less(t, time.Since(started), 50*time.Millisecond)
	require.Eventually(t, func() bool {
		health := blocker.Health()
		return health.Failures >= 1 && health.Applied == 1 && health.ActiveRules == 1
	}, 4*time.Second, 20*time.Millisecond)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, 2, fake.createCalls)
}

func TestCloudflareIngressBlockerReconcilesExpiredManagedRule(t *testing.T) {
	fake := newCloudflareFakeAPI()
	fake.rules["stale-rule"] = cloudflareAccessRule{
		ID: "stale-rule", Mode: "block",
		Configuration: cloudflareAccessRuleConfiguration{Target: "ip", Value: "192.0.2.30"},
		Notes:         cloudflareRuleNote(time.Now().Add(-time.Minute)),
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	blocker := cloudflareBlockerForTest(server, fake)
	blocker.Start()
	t.Cleanup(blocker.Stop)

	require.Eventually(t, func() bool {
		return blocker.Health().Released == 1
	}, 2*time.Second, 10*time.Millisecond)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.rules)
}

func TestCloudflareIngressBlockerHonorsRemoteExtensionBeforeRelease(t *testing.T) {
	fake := newCloudflareFakeAPI()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	blocker := cloudflareBlockerForTest(server, fake)
	blocker.Start()
	t.Cleanup(blocker.Stop)

	firstExpiry := time.Now().Add(120 * time.Millisecond)
	require.True(t, blocker.EnqueueBlock("203.0.113.50", firstExpiry))
	require.Eventually(t, func() bool { return blocker.Health().ActiveRules == 1 }, 2*time.Second, 10*time.Millisecond)

	remoteExpiry := time.Now().Add(350 * time.Millisecond)
	fake.mu.Lock()
	for id, rule := range fake.rules {
		rule.Notes = cloudflareRuleNote(remoteExpiry)
		fake.rules[id] = rule
	}
	fake.mu.Unlock()

	time.Sleep(time.Until(firstExpiry) + 40*time.Millisecond)
	require.Equal(t, 1, blocker.Health().ActiveRules)
	fake.mu.Lock()
	require.Equal(t, 0, fake.deleteCalls)
	fake.mu.Unlock()
	require.Eventually(t, func() bool { return blocker.Health().ActiveRules == 0 }, 2*time.Second, 10*time.Millisecond)
}

func TestCloudflareIngressBlockerCleanupOnlyModeDoesNotAcceptNewBlocks(t *testing.T) {
	fake := newCloudflareFakeAPI()
	fake.rules["stale-rule"] = cloudflareAccessRule{
		ID: "stale-rule", Mode: "block",
		Configuration: cloudflareAccessRuleConfiguration{Target: "ip", Value: "192.0.2.60"},
		Notes:         cloudflareRuleNote(time.Now().Add(-time.Minute)),
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := service.CloudflareIngressSettings{
		Enabled: false, APIToken: "secret-token", ZoneID: "test-zone",
		QueueCapacity: 16, MaxActiveRules: 8, ReconcileIntervalSeconds: 3600, RequestTimeoutSeconds: 1,
	}
	client := newCloudflareIngressClient(server.URL, "test-zone", "secret-token", time.Second)
	blocker := newCloudflareIngressBlocker(settings, client)
	blocker.pollInterval = 10 * time.Millisecond
	blocker.Start()
	t.Cleanup(blocker.Stop)

	require.False(t, blocker.EnqueueBlock("192.0.2.61", time.Now().Add(time.Minute)))
	require.Eventually(t, func() bool { return blocker.Health().Released == 1 }, 2*time.Second, 10*time.Millisecond)
	require.False(t, blocker.Health().Enabled)
	require.Eventually(t, func() bool { return !blocker.Health().Running }, 2*time.Second, 10*time.Millisecond)
}

func TestCloudflareIngressBlockerHotAppliesSettingsWithoutRestart(t *testing.T) {
	fake := newCloudflareFakeAPI()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := service.CloudflareIngressSettings{
		Enabled: false, APIToken: "secret-token", ZoneID: "test-zone",
		QueueCapacity: 16, MaxActiveRules: 8, ReconcileIntervalSeconds: 3600, RequestTimeoutSeconds: 1,
	}
	client := newCloudflareIngressClient(server.URL, "test-zone", "secret-token", time.Second)
	blocker := newCloudflareIngressBlocker(settings, client)
	blocker.pollInterval = 10 * time.Millisecond
	blocker.Start()
	t.Cleanup(blocker.Stop)

	require.False(t, blocker.EnqueueBlock("192.0.2.70", time.Now().Add(time.Minute)))
	settings.Enabled = true
	require.NoError(t, blocker.ApplyCloudflareIngressSettings(t.Context(), settings))
	require.Eventually(t, func() bool { return blocker.Health().Enabled && blocker.Health().Running }, 2*time.Second, 10*time.Millisecond)
	require.True(t, blocker.EnqueueBlock("192.0.2.70", time.Now().Add(150*time.Millisecond)))
	require.Eventually(t, func() bool { return blocker.Health().ActiveRules == 1 }, 2*time.Second, 10*time.Millisecond)

	settings.Enabled = false
	require.NoError(t, blocker.ApplyCloudflareIngressSettings(t.Context(), settings))
	require.False(t, blocker.EnqueueBlock("192.0.2.71", time.Now().Add(time.Minute)))
	require.Eventually(t, func() bool {
		health := blocker.Health()
		return !health.Enabled && !health.Running && health.ActiveRules == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestCloudflareBlockTargetUsesIPv6PrefixAndRejectsPrivateAddresses(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute)
	request, ok := cloudflareBlockTarget("2001:db8:abcd:1234::", expiresAt)
	require.True(t, ok)
	require.Equal(t, "ip_range", request.target)
	require.Equal(t, "2001:db8:abcd:1234::/64", request.value)

	_, ok = cloudflareBlockTarget("10.0.0.1", expiresAt)
	require.False(t, ok)
	_, ok = cloudflareBlockTarget("::1", expiresAt)
	require.False(t, ok)
}

func TestCloudflareIngressBlockerQueueIsBounded(t *testing.T) {
	fake := newCloudflareFakeAPI()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	settings := service.CloudflareIngressSettings{
		Enabled: true, QueueCapacity: service.CloudflareIngressMinQueueCapacity, MaxActiveRules: 1,
		ReconcileIntervalSeconds: 3600, RequestTimeoutSeconds: 1,
	}
	client := newCloudflareIngressClient(server.URL, "test-zone", "secret-token", time.Second)
	blocker := newCloudflareIngressBlocker(settings, client)
	t.Cleanup(blocker.Stop)

	for index := 1; index <= service.CloudflareIngressMinQueueCapacity; index++ {
		require.True(t, blocker.EnqueueBlock(fmt.Sprintf("192.0.2.%d", index), time.Now().Add(time.Minute)))
	}
	require.False(t, blocker.EnqueueBlock("192.0.2.200", time.Now().Add(time.Minute)))
	require.Equal(t, uint64(1), blocker.Health().Dropped)
}

func TestCloudflareIngressClientFollowsReportedPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		page := request.URL.Query().Get("page")
		rule := cloudflareAccessRule{
			ID: "rule-" + page, Mode: "block",
			Configuration: cloudflareAccessRuleConfiguration{Target: "ip", Value: "192.0.2." + page},
			Notes:         cloudflareRuleNote(time.Now().Add(time.Minute)),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "errors": []any{}, "result": []cloudflareAccessRule{rule},
			"result_info": map[string]any{"count": 1, "page": requests, "per_page": 1, "total_count": 2},
		})
	}))
	t.Cleanup(server.Close)
	client := newCloudflareIngressClient(server.URL, "test-zone", "secret-token", time.Second)

	rules, err := client.listManagedRules(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, rules, 2)
	require.Equal(t, 2, requests)
}
