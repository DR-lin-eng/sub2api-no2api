//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthMultimodalVideoReadsAfterFullBalanceReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		platform    string
		method      string
		path        string
		keyStatus   string
		userStatus  string
		groupStatus string
		want        int
	}{
		{name: "status read", path: "/v1/videos/task-1", want: http.StatusOK},
		{name: "content read", path: "/v1/videos/task-1/content", want: http.StatusOK},
		{name: "new generation remains blocked", method: http.MethodPost, path: "/v1/videos/generations", want: http.StatusForbidden},
		{name: "other platforms retain existing billing admission", platform: service.PlatformGrok, path: "/v1/videos/task-1", want: http.StatusForbidden},
		{name: "disabled key remains blocked", path: "/v1/videos/task-1/content", keyStatus: service.StatusDisabled, want: http.StatusUnauthorized},
		{name: "expired key remains blocked", path: "/v1/videos/task-1", keyStatus: service.StatusAPIKeyExpired, want: http.StatusForbidden},
		{name: "quota exhausted key remains blocked", path: "/v1/videos/task-1", keyStatus: service.StatusAPIKeyQuotaExhausted, want: http.StatusTooManyRequests},
		{name: "inactive user remains blocked", path: "/v1/videos/task-1", userStatus: service.StatusDisabled, want: http.StatusUnauthorized},
		{name: "disabled group remains blocked", path: "/v1/videos/task-1", groupStatus: service.StatusDisabled, want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &service.User{ID: 12, Role: service.RoleUser, Status: service.StatusActive, Balance: 0, FrozenBalance: 1, Concurrency: 3}
			group := &service.Group{ID: 31, Platform: service.PlatformMultimodal, Status: service.StatusActive, Hydrated: true}
			key := &service.APIKey{ID: 101, UserID: user.ID, Key: "held-video", Status: service.StatusActive, User: user, GroupID: &group.ID, Group: group}
			if tc.platform != "" {
				group.Platform = tc.platform
			}
			if tc.keyStatus != "" {
				key.Status = tc.keyStatus
			}
			if tc.userStatus != "" {
				user.Status = tc.userStatus
			}
			if tc.groupStatus != "" {
				group.Status = tc.groupStatus
			}
			repo := &stubApiKeyRepo{getByKey: func(_ context.Context, input string) (*service.APIKey, error) {
				if input != key.Key {
					return nil, service.ErrAPIKeyNotFound
				}
				return key, nil
			}}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			router.Handle(method, tc.path, func(c *gin.Context) {
				actual, ok := GetAPIKeyFromContext(c)
				require.True(t, ok)
				require.Equal(t, key.ID, actual.ID)
				c.Status(http.StatusOK)
			})
			request := httptest.NewRequest(method, tc.path, nil)
			request.Header.Set("Authorization", "Bearer "+key.Key)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
		})
	}
}
