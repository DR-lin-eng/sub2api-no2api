//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelMonitorPublicSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
}

func (s *channelMonitorPublicSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return s.values, nil
}

func newPublicChannelMonitorTestContext(t *testing.T, userID *int64) (*ChannelMonitorUserHandler, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&channelMonitorPublicSettingRepoStub{values: map[string]string{
		service.SettingKeyChannelMonitorEnabled:                "true",
		service.SettingKeyChannelMonitorPublicShareEnabled:     "true",
		service.SettingKeyChannelMonitorPublicShareRequireAuth: "true",
	}}, &config.Config{})
	h := NewChannelMonitorUserHandler(nil, settingService)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/channel-status-share", nil)
	if userID != nil {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: *userID})
	}
	return h, c, recorder
}

func TestChannelMonitorPublicShareRequireAuthRejectsMissingOrInvalidSubject(t *testing.T) {
	zero := int64(0)
	for _, tt := range []struct {
		name   string
		userID *int64
	}{
		{name: "missing subject"},
		{name: "zero user id", userID: &zero},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, c, recorder := newPublicChannelMonitorTestContext(t, tt.userID)

			require.False(t, h.publicShareAllowed(c))
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

func TestChannelMonitorPublicShareRequireAuthAcceptsPositiveUserID(t *testing.T) {
	userID := int64(42)
	h, c, recorder := newPublicChannelMonitorTestContext(t, &userID)

	require.True(t, h.publicShareAllowed(c))
	require.Equal(t, http.StatusOK, recorder.Code)
}
