package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenCookieTTLEnforcesSevenDayMinimum(t *testing.T) {
	h := &AuthHandler{cfg: &config.Config{JWT: config.JWTConfig{RefreshTokenExpireDays: 2}}}
	require.Equal(t, 7*24*time.Hour, h.refreshTokenCookieTTL())

	h.cfg.JWT.RefreshTokenExpireDays = 14
	require.Equal(t, 14*24*time.Hour, h.refreshTokenCookieTTL())
}
