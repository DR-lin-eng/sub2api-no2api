package service

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const openAILocationSettingsCacheTTL = 60 * time.Second

type cachedOpenAILocationSettings struct {
	enabled   bool
	latitude  float64
	longitude float64
	expiresAt time.Time
}

// ValidateOpenAILocation validates the administrator-provided approximate
// coordinate. The gateway never accepts a coordinate outside the WGS84 range.
func ValidateOpenAILocation(latitude, longitude float64) error {
	if math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 {
		return infraerrors.BadRequest("INVALID_OPENAI_LOCATION", "OpenAI location latitude must be between -90 and 90")
	}
	if math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 {
		return infraerrors.BadRequest("INVALID_OPENAI_LOCATION", "OpenAI location longitude must be between -180 and 180")
	}
	return nil
}

func (s *SettingService) resolveOpenAILocationSettings(values map[string]string) (bool, float64, float64) {
	latitude, latErr := strconv.ParseFloat(strings.TrimSpace(values[SettingKeyOpenAILocationLatitude]), 64)
	longitude, lonErr := strconv.ParseFloat(strings.TrimSpace(values[SettingKeyOpenAILocationLongitude]), 64)
	if latErr != nil || lonErr != nil || ValidateOpenAILocation(latitude, longitude) != nil {
		return false, 0, 0
	}
	return strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAILocationEnabled]), "true"), latitude, longitude
}

// GetOpenAILocationSettings returns the hot-path location policy. It is shared
// by HTTP, Images and WebSocket request builders and refreshed after an admin
// settings write, so a process does not query PostgreSQL for every request.
func (s *SettingService) GetOpenAILocationSettings(ctx context.Context) (bool, float64, float64) {
	if s == nil {
		return false, 0, 0
	}
	if cached := s.openAILocationCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled, cached.latitude, cached.longitude
	}
	s.openAILocationSettingsMu.Lock()
	defer s.openAILocationSettingsMu.Unlock()
	if cached := s.openAILocationCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled, cached.latitude, cached.longitude
	}
	if s.settingRepo == nil {
		return false, 0, 0
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(readCtx, []string{
		SettingKeyOpenAILocationEnabled,
		SettingKeyOpenAILocationLatitude,
		SettingKeyOpenAILocationLongitude,
	})
	if err != nil {
		return false, 0, 0
	}
	enabled, latitude, longitude := s.resolveOpenAILocationSettings(values)
	s.openAILocationCache.Store(&cachedOpenAILocationSettings{
		enabled: enabled, latitude: latitude, longitude: longitude,
		expiresAt: time.Now().Add(openAILocationSettingsCacheTTL),
	})
	return enabled, latitude, longitude
}

func (s *SettingService) refreshOpenAILocationSettings(settings *SystemSettings) {
	if s == nil || settings == nil {
		return
	}
	s.openAILocationSettingsMu.Lock()
	defer s.openAILocationSettingsMu.Unlock()
	s.openAILocationCache.Store(&cachedOpenAILocationSettings{
		enabled: settings.OpenAILocationEnabled, latitude: settings.OpenAILocationLatitude,
		longitude: settings.OpenAILocationLongitude,
		expiresAt: time.Now().Add(openAILocationSettingsCacheTTL),
	})
}
