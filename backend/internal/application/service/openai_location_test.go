package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestOpenAILocationHeaderUsesPersistentOffsetAndTenMinuteCache(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyOpenAILocationEnabled:   "true",
		SettingKeyOpenAILocationLatitude:  "31.2304",
		SettingKeyOpenAILocationLongitude: "121.4737",
	}}
	settings := NewSettingService(repo, &config.Config{})
	offsetRepo := &openAILocationMemoryRepo{}
	gateway := &OpenAIGatewayService{settingService: settings, accountRepo: offsetRepo}
	account := &Account{ID: 42, Platform: PlatformOpenAI}

	first := make(http.Header)
	first.Set("OAI-Location", " forged")
	gateway.applyOpenAILocationHeader(context.Background(), first, account)
	lat1, lon1, accuracy1 := parseOpenAILocationHeader(t, first.Get("OAI-Location"))
	require.GreaterOrEqual(t, accuracy1, 3.0)
	require.LessOrEqual(t, accuracy1, 20.0)
	offset, ok := readOpenAILocationPersistentOffset(map[string]any{OpenAILocationPersistentOffsetExtraKey: offsetRepo.offset})
	require.True(t, ok)
	require.LessOrEqual(t, math.Hypot(offset.NorthMeters, offset.EastMeters), openAILocationPersistentRadiusMeters)
	require.LessOrEqual(t, haversineMeters(31.2304, 121.4737, lat1, lon1), openAILocationPersistentRadiusMeters+openAILocationCacheRadiusMeters+1)

	second := make(http.Header)
	gateway.applyOpenAILocationHeader(context.Background(), second, account)
	require.Equal(t, first.Get("OAI-Location"), second.Get("OAI-Location"))

	gateway.openAILocationMu.Lock()
	entry := gateway.openAILocationCache[openAILocationAccountKey(account)]
	entry.expiresAt = time.Now().Add(-time.Second)
	gateway.openAILocationCache[openAILocationAccountKey(account)] = entry
	gateway.openAILocationMu.Unlock()

	third := make(http.Header)
	gateway.applyOpenAILocationHeader(context.Background(), third, account)
	_, _, accuracy3 := parseOpenAILocationHeader(t, third.Get("OAI-Location"))
	require.GreaterOrEqual(t, accuracy3, 3.0)
	require.LessOrEqual(t, accuracy3, 20.0)
	// The offset is persisted on the account while the 10m cache is replaced.
	require.Equal(t, offset, offsetRepo.offset)
}

func TestOpenAILocationDisabledRemovesInboundHeaderWithoutGeneratingOffset(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyOpenAILocationEnabled:   "false",
		SettingKeyOpenAILocationLatitude:  "31.2304",
		SettingKeyOpenAILocationLongitude: "121.4737",
	}}
	gateway := &OpenAIGatewayService{settingService: NewSettingService(repo, nil)}
	account := &Account{ID: 7, Platform: PlatformOpenAI}
	headers := http.Header{"OAI-Location": []string{"1,2,3"}}
	gateway.applyOpenAILocationHeader(context.Background(), headers, account)
	require.Empty(t, headers.Get("OAI-Location"))
	require.Empty(t, account.Extra)
}

func TestValidateOpenAILocation(t *testing.T) {
	require.NoError(t, ValidateOpenAILocation(90, -180))
	require.NoError(t, ValidateOpenAILocation(-90, 180))
	require.Error(t, ValidateOpenAILocation(90.1, 0))
	require.Error(t, ValidateOpenAILocation(0, -180.1))
	require.Error(t, ValidateOpenAILocation(math.NaN(), 0))
}

func TestOpenAILocationSettingsPersistAndRejectInvalidEnabledCoordinate(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	view := settings.parseSettings(repo.data)
	view.OpenAILocationEnabled = true
	view.OpenAILocationLatitude = 31.2304
	view.OpenAILocationLongitude = 121.4737
	require.NoError(t, settings.UpdateSettings(context.Background(), view))
	require.Equal(t, "true", repo.data[SettingKeyOpenAILocationEnabled])
	require.Equal(t, "31.2304", repo.data[SettingKeyOpenAILocationLatitude])
	require.Equal(t, "121.4737", repo.data[SettingKeyOpenAILocationLongitude])

	view.OpenAILocationLatitude = 91
	require.Error(t, settings.UpdateSettings(context.Background(), view))
	require.Equal(t, "31.2304", repo.data[SettingKeyOpenAILocationLatitude])
}

func TestOpenAILocationIsRebuiltAtHTTPUpstreamBoundary(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyOpenAILocationEnabled:   "true",
		SettingKeyOpenAILocationLatitude:  "31.2304",
		SettingKeyOpenAILocationLongitude: "121.4737",
	}}
	upstream := &openAILocationHTTPUpstreamCapture{}
	gateway := &OpenAIGatewayService{
		settingService: NewSettingService(repo, nil),
		httpUpstream:   upstream,
		accountRepo:    &openAILocationMemoryRepo{},
	}
	account := &Account{ID: 19, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	req, err := http.NewRequest(http.MethodPost, "https://example.test/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("OAI-Location", "1,2,3")
	_, err = gateway.doAccountHTTPUpstream(req, "", account)
	require.NoError(t, err)
	require.NotNil(t, upstream.request)
	_, _, accuracy := parseOpenAILocationHeader(t, upstream.request.Header.Get("OAI-Location"))
	require.GreaterOrEqual(t, accuracy, 3.0)
	require.LessOrEqual(t, accuracy, 20.0)
}

type openAILocationHTTPUpstreamCapture struct {
	request *http.Request
}

func (u *openAILocationHTTPUpstreamCapture) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.request = req.Clone(req.Context())
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func (u *openAILocationHTTPUpstreamCapture) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func parseOpenAILocationHeader(t *testing.T, raw string) (float64, float64, float64) {
	t.Helper()
	parts := strings.Split(raw, ",")
	require.Len(t, parts, 3)
	latitude, err := strconv.ParseFloat(parts[0], 64)
	require.NoError(t, err)
	longitude, err := strconv.ParseFloat(parts[1], 64)
	require.NoError(t, err)
	accuracy, err := strconv.ParseFloat(parts[2], 64)
	require.NoError(t, err)
	return latitude, longitude, accuracy
}

func mustReadOpenAILocationPersistentOffset(t *testing.T, extra map[string]any) openAILocationPersistentOffset {
	t.Helper()
	offset, ok := readOpenAILocationPersistentOffset(extra)
	require.True(t, ok)
	return offset
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const radius = 6371008.8
	lat1, lon1, lat2, lon2 = lat1*math.Pi/180, lon1*math.Pi/180, lat2*math.Pi/180, lon2*math.Pi/180
	dLat, dLon := lat2-lat1, lon2-lon1
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return radius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// This stub models an atomic durable store. Request snapshots remain immutable.
type openAILocationMemoryRepo struct {
	AccountRepository
	mu     sync.Mutex
	offset OpenAILocationOffset
	ready  bool
	calls  int
	err    error
}

func (r *openAILocationMemoryRepo) GetOrCreateOpenAILocationOffset(_ context.Context, _ int64, candidate OpenAILocationOffset) (OpenAILocationOffset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return OpenAILocationOffset{}, r.err
	}
	if !r.ready {
		r.offset = candidate
		r.ready = true
	}
	return r.offset, nil
}

func TestOpenAILocationConcurrentRequestsKeepSnapshotImmutable(t *testing.T) {
	settings := NewSettingService(nil, nil)
	settings.refreshOpenAILocationSettings(&SystemSettings{OpenAILocationEnabled: true, OpenAILocationLatitude: 80, OpenAILocationLongitude: 179.999})
	repo := &openAILocationMemoryRepo{}
	gateway := &OpenAIGatewayService{settingService: settings, accountRepo: repo}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Extra: map[string]any{"keep": true}}
	var wg sync.WaitGroup
	values := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := make(http.Header)
			gateway.applyOpenAILocationHeader(context.Background(), h, account)
			values <- h.Get("OAI-Location")
		}()
	}
	wg.Wait()
	close(values)
	first := ""
	for value := range values {
		require.NotEmpty(t, value)
		if first == "" {
			first = value
		}
		require.Equal(t, first, value)
	}
	require.Equal(t, 1, repo.calls)
	require.Equal(t, map[string]any{"keep": true}, account.Extra)
}

func TestOpenAILocationPersistenceFailureDoesNotSubmitAndRetries(t *testing.T) {
	settings := NewSettingService(nil, nil)
	settings.refreshOpenAILocationSettings(&SystemSettings{OpenAILocationEnabled: true})
	repo := &openAILocationMemoryRepo{err: errors.New("fixture persistence unavailable")}
	gateway := &OpenAIGatewayService{settingService: settings, accountRepo: repo}
	account := &Account{ID: 42, Platform: PlatformOpenAI}
	h := make(http.Header)
	gateway.applyOpenAILocationHeader(context.Background(), h, account)
	require.Empty(t, h.Get("OAI-Location"))
	require.Empty(t, gateway.openAILocationCache)
	repo.err = nil
	gateway.applyOpenAILocationHeader(context.Background(), h, account)
	require.NotEmpty(t, h.Get("OAI-Location"))
	require.Equal(t, 2, repo.calls)
	h.Set("OAI-Location", "fixture")
	gateway.applyOpenAILocationHeader(WithHTTPUpstreamPublicDestination(context.Background()), h, account)
	require.Empty(t, h.Get("OAI-Location"))
}

func TestOpenAILocationSphericalRadiusAndWSCompatibility(t *testing.T) {
	for _, lat := range []float64{-90, -89.999, 0, 80, 89.999, 90} {
		for _, lon := range []float64{-180, 0, 179.999} {
			anchor := coordinateFromMeters(lat, lon, 300, 400)
			require.InDelta(t, 500, haversineMeters(lat, lon, anchor.latitude, anchor.longitude), 0.001)
			shifted := coordinateFromMeters(anchor.latitude, anchor.longitude, 6, 8)
			require.InDelta(t, 10, haversineMeters(anchor.latitude, anchor.longitude, shifted.latitude, shifted.longitude), 0.01)
		}
	}
	first := http.Header{}
	first.Set("OAI-Location", "1,2,3")
	second := first.Clone()
	second.Set("OAI-Location", "1,2,4")
	require.NotEqual(t, normalizeOpenAIWSHandshakeCompatibility(first), normalizeOpenAIWSHandshakeCompatibility(second))
}
