package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestIsReservedEmail_DingTalkDomain(t *testing.T) {
	require.True(t, isReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.True(t, isReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID")) // case-insensitive
	require.False(t, isReservedEmail("real@dingtalk.com"))
}

func TestRefreshTokenTTLEnforcesSevenDaySessionMinimum(t *testing.T) {
	svc := &AuthService{cfg: &config.Config{JWT: config.JWTConfig{RefreshTokenExpireDays: 1}}}
	require.Equal(t, 7*24*time.Hour, svc.refreshTokenTTL())

	svc.cfg.JWT.RefreshTokenExpireDays = 30
	require.Equal(t, 30*24*time.Hour, svc.refreshTokenTTL())
}
