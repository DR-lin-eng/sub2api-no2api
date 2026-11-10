package service

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// OpenAILocationPersistentOffsetExtraKey is stored in an account's JSONB
	// extra document. It contains the account's stable displacement from the
	// administrator's approximate coordinate in meters.
	OpenAILocationPersistentOffsetExtraKey = "openai_location_persistent_offset"
	openAILocationPersistentRadiusMeters   = 500.0
	openAILocationCacheRadiusMeters        = 10.0
	openAILocationCacheTTL                 = 10 * time.Minute
	openAILocationEarthRadiusMeters        = 6371008.8
)

type openAILocationCacheEntry struct {
	latitude      float64
	longitude     float64
	baseLatitude  float64
	baseLongitude float64
	accuracy      float64
	expiresAt     time.Time
}

// applyOpenAILocationHeader rebuilds OAI-Location at the last possible point
// before an OpenAI request leaves the gateway. The inbound header is always
// removed first, so clients and API-key header overrides cannot bypass the
// administrator policy. The short-lived offset is created lazily on a real
// request; there is no background location generation.
func (s *OpenAIGatewayService) applyOpenAILocationHeader(ctx context.Context, headers http.Header, account *Account) {
	if headers == nil {
		return
	}
	deleteOpenAIHeaderEqualFold(headers, "OAI-Location")
	if HTTPUpstreamPublicDestination(ctx) {
		return
	}
	if account == nil || (account.Platform != "" && account.Platform != PlatformOpenAI) || s == nil || s.settingService == nil {
		return
	}
	enabled, baseLatitude, baseLongitude := s.settingService.GetOpenAILocationSettings(ctx)
	if !enabled {
		s.openAILocationMu.Lock()
		delete(s.openAILocationCache, openAILocationAccountKey(account))
		s.openAILocationMu.Unlock()
		return
	}
	key := openAILocationAccountKey(account)
	now := time.Now()
	if value, ok := s.cachedOpenAILocation(key, baseLatitude, baseLongitude, now); ok {
		headers.Set("OAI-Location", formatOpenAILocationHeader(value.latitude, value.longitude, value.accuracy))
		return
	}
	value, err, _ := s.openAILocationSF.Do(key, func() (any, error) {
		if value, ok := s.cachedOpenAILocation(key, baseLatitude, baseLongitude, time.Now()); ok {
			return value, nil
		}
		persistent, err := s.ensureOpenAILocationPersistentOffset(ctx, account)
		if err != nil {
			return nil, err
		}
		return s.openAILocationForRequest(ctx, account, baseLatitude, baseLongitude, persistent)
	})
	if err != nil {
		logOpenAILocationPersistenceFailure(account.ID, err)
		return
	}
	coordinate, ok := value.(openAILocationCoordinate)
	if !ok {
		return
	}
	headers.Set("OAI-Location", formatOpenAILocationHeader(coordinate.latitude, coordinate.longitude, coordinate.accuracy))
}

type openAILocationCoordinate struct {
	latitude  float64
	longitude float64
	accuracy  float64
}

func formatOpenAILocationHeader(latitude, longitude, accuracy float64) string {
	return strings.Join([]string{
		strconv.FormatFloat(latitude, 'f', -1, 64),
		strconv.FormatFloat(longitude, 'f', -1, 64),
		strconv.FormatFloat(accuracy, 'f', -1, 64),
	}, ",")
}

// OpenAILocationOffsetRepository fixes the first offset atomically across replicas.
// A failed persistence attempt must not submit a temporary, unstable location.
type OpenAILocationOffsetRepository interface {
	GetOrCreateOpenAILocationOffset(context.Context, int64, OpenAILocationOffset) (OpenAILocationOffset, error)
}

type OpenAILocationOffset struct {
	NorthMeters float64 `json:"north_meters"`
	EastMeters  float64 `json:"east_meters"`
}

type openAILocationPersistentOffset = OpenAILocationOffset

func (s *OpenAIGatewayService) ensureOpenAILocationPersistentOffset(ctx context.Context, account *Account) (openAILocationPersistentOffset, error) {
	if repo, ok := s.accountRepo.(OpenAILocationOffsetRepository); ok && account.ID > 0 {
		persistCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return repo.GetOrCreateOpenAILocationOffset(persistCtx, account.ID, generateOpenAILocationPersistentOffset(account.ID))
	}
	// Lightweight embedding callers can provide a previously persisted snapshot.
	if offset, ok := readOpenAILocationPersistentOffset(account.Extra); ok {
		return offset, nil
	}
	return OpenAILocationOffset{}, fmt.Errorf("location offset persistence is unavailable")
}

func openAILocationAccountKey(account *Account) string {
	if account.ID > 0 {
		return strconv.FormatInt(account.ID, 10)
	}
	return fmt.Sprintf("ptr:%p", account)
}

func readOpenAILocationPersistentOffset(extra map[string]any) (openAILocationPersistentOffset, bool) {
	if extra == nil {
		return openAILocationPersistentOffset{}, false
	}
	raw, ok := extra[OpenAILocationPersistentOffsetExtraKey]
	if !ok || raw == nil {
		return openAILocationPersistentOffset{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return openAILocationPersistentOffset{}, false
	}
	var offset openAILocationPersistentOffset
	if err := json.Unmarshal(encoded, &offset); err != nil || !validOpenAILocationPersistentOffset(offset) {
		return openAILocationPersistentOffset{}, false
	}
	return offset, true
}

func validOpenAILocationPersistentOffset(offset openAILocationPersistentOffset) bool {
	if math.IsNaN(offset.NorthMeters) || math.IsInf(offset.NorthMeters, 0) ||
		math.IsNaN(offset.EastMeters) || math.IsInf(offset.EastMeters, 0) {
		return false
	}
	return math.Hypot(offset.NorthMeters, offset.EastMeters) <= openAILocationPersistentRadiusMeters+1e-6
}

func generateOpenAILocationPersistentOffset(accountID int64) openAILocationPersistentOffset {
	u1, ok1 := secureOpenAILocationUnit()
	u2, ok2 := secureOpenAILocationUnit()
	if !ok1 || !ok2 {
		seed := sha256.Sum256([]byte(fmt.Sprintf("openai-location:%d:%d", accountID, time.Now().UnixNano())))
		u1 = float64(binary.LittleEndian.Uint64(seed[:8])) / float64(^uint64(0))
		u2 = float64(binary.LittleEndian.Uint64(seed[8:16])) / float64(^uint64(0))
	}
	radius := openAILocationPersistentRadiusMeters * math.Sqrt(clampUnitInterval(u1))
	angle := 2 * math.Pi * clampUnitInterval(u2)
	return openAILocationPersistentOffset{
		NorthMeters: radius * math.Cos(angle),
		EastMeters:  radius * math.Sin(angle),
	}
}

func secureOpenAILocationUnit() (float64, bool) {
	var raw [8]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return 0, false
	}
	return float64(binary.LittleEndian.Uint64(raw[:])) / float64(^uint64(0)), true
}

func clampUnitInterval(value float64) float64 {
	if value <= 0 {
		return math.SmallestNonzeroFloat64
	}
	if value >= 1 {
		return math.Nextafter(1, 0)
	}
	return value
}

func (s *OpenAIGatewayService) cachedOpenAILocation(key string, latitude, longitude float64, now time.Time) (openAILocationCoordinate, bool) {
	s.openAILocationMu.Lock()
	defer s.openAILocationMu.Unlock()
	cached, ok := s.openAILocationCache[key]
	if !ok || !now.Before(cached.expiresAt) || cached.baseLatitude != latitude || cached.baseLongitude != longitude {
		return openAILocationCoordinate{}, false
	}
	return openAILocationCoordinate{cached.latitude, cached.longitude, cached.accuracy}, true
}

// OpenAILocationCache shares one fixed ten-minute sample across replicas.
type OpenAILocationCache interface {
	GetOrCreateOpenAILocation(context.Context, string, string, time.Duration) (string, time.Duration, error)
}

func (s *OpenAIGatewayService) openAILocationForRequest(ctx context.Context, account *Account, baseLatitude, baseLongitude float64, persistent openAILocationPersistentOffset) (openAILocationCoordinate, error) {
	key := openAILocationAccountKey(account)
	now := time.Now()
	u1, ok1 := secureOpenAILocationUnit()
	u2, ok2 := secureOpenAILocationUnit()
	u3, ok3 := secureOpenAILocationUnit()
	if !ok1 || !ok2 || !ok3 {
		seed := sha256.Sum256([]byte(fmt.Sprintf("openai-location-cache:%s:%d", key, now.UnixNano())))
		u1 = float64(binary.LittleEndian.Uint64(seed[:8])) / float64(^uint64(0))
		u2 = float64(binary.LittleEndian.Uint64(seed[8:16])) / float64(^uint64(0))
		u3 = float64(binary.LittleEndian.Uint64(seed[16:24])) / float64(^uint64(0))
	}
	radius := openAILocationCacheRadiusMeters * math.Sqrt(clampUnitInterval(u1))
	angle := 2 * math.Pi * clampUnitInterval(u2)
	anchor := coordinateFromMeters(baseLatitude, baseLongitude, persistent.NorthMeters, persistent.EastMeters)
	coordinate := coordinateFromMeters(anchor.latitude, anchor.longitude, radius*math.Cos(angle), radius*math.Sin(angle))
	coordinate.accuracy = 3 + math.Floor(clampUnitInterval(u3)*18)
	ttl := openAILocationCacheTTL
	if shared, ok := s.cache.(OpenAILocationCache); ok {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%g:%g:%g:%g", key, baseLatitude, baseLongitude, persistent.NorthMeters, persistent.EastMeters)))
		cacheCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		value, remaining, err := shared.GetOrCreateOpenAILocation(cacheCtx, fmt.Sprintf("%x", digest), formatOpenAILocationHeader(coordinate.latitude, coordinate.longitude, coordinate.accuracy), ttl)
		cancel()
		if err != nil {
			return openAILocationCoordinate{}, err
		}
		parts := strings.Split(value, ",")
		if len(parts) != 3 {
			return openAILocationCoordinate{}, fmt.Errorf("invalid shared location")
		}
		lat, e1 := strconv.ParseFloat(parts[0], 64)
		lon, e2 := strconv.ParseFloat(parts[1], 64)
		acc, e3 := strconv.ParseFloat(parts[2], 64)
		if e1 != nil || e2 != nil || e3 != nil || ValidateOpenAILocation(lat, lon) != nil || math.IsNaN(acc) || acc < 3 || acc > 20 || remaining <= 0 {
			return openAILocationCoordinate{}, fmt.Errorf("invalid shared location")
		}
		coordinate = openAILocationCoordinate{lat, lon, acc}
		ttl = min(ttl, remaining)
	}
	s.openAILocationMu.Lock()
	defer s.openAILocationMu.Unlock()
	if s.openAILocationCache == nil {
		s.openAILocationCache = make(map[string]openAILocationCacheEntry)
	}
	if len(s.openAILocationCache) >= 65536 {
		clear(s.openAILocationCache)
	}
	s.openAILocationCache[key] = openAILocationCacheEntry{
		latitude: coordinate.latitude, longitude: coordinate.longitude,
		baseLatitude: baseLatitude, baseLongitude: baseLongitude,
		accuracy:  coordinate.accuracy,
		expiresAt: now.Add(ttl),
	}
	return coordinate, nil
}

func coordinateFromMeters(baseLatitude, baseLongitude, northMeters, eastMeters float64) openAILocationCoordinate {
	distance := math.Hypot(northMeters, eastMeters) / openAILocationEarthRadiusMeters
	bearing := math.Atan2(eastMeters, northMeters)
	phi, lambda := baseLatitude*math.Pi/180, baseLongitude*math.Pi/180
	latitude := math.Asin(math.Max(-1, math.Min(1, math.Sin(phi)*math.Cos(distance)+math.Cos(phi)*math.Sin(distance)*math.Cos(bearing))))
	longitude := lambda + math.Atan2(math.Sin(bearing)*math.Sin(distance)*math.Cos(phi), math.Cos(distance)-math.Sin(phi)*math.Sin(latitude))
	longitude = math.Mod(longitude*180/math.Pi+540, 360) - 180
	return openAILocationCoordinate{latitude: latitude * 180 / math.Pi, longitude: longitude}
}

func logOpenAILocationPersistenceFailure(accountID int64, err error) {
	slog.Warn("openai location offset persistence failed", "account_id", accountID, "error", err)
}
