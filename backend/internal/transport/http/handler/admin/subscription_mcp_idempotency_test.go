package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	middleware "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mcpAssignGroups struct {
	service.GroupRepository
	reads int
}

func (r *mcpAssignGroups) GetByID(context.Context, int64) (*service.Group, error) {
	r.reads++
	return &service.Group{ID: 4, SubscriptionType: service.SubscriptionTypeSubscription}, nil
}

type mcpAssignSubscriptions struct {
	service.UserSubscriptionRepository
	sub *service.UserSubscription
}

func (r *mcpAssignSubscriptions) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return r.sub != nil, nil
}
func (r *mcpAssignSubscriptions) Create(_ context.Context, sub *service.UserSubscription) error {
	sub.ID = 11
	r.sub = sub
	return nil
}
func (r *mcpAssignSubscriptions) GetByID(context.Context, int64) (*service.UserSubscription, error) {
	return r.sub, nil
}

func TestMCPAssignmentRetriesReplayAndBrowserNeedsNoNewHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	for _, method := range []string{service.AuditAuthMethodJWT, service.AuditAuthMethodMCPOAuth} {
		t.Run(method, func(t *testing.T) {
			policy := service.DefaultIdempotencyConfig()
			policy.ObserveOnly = false
			service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), policy))
			groups := &mcpAssignGroups{}
			h := NewSubscriptionHandler(service.NewSubscriptionService(groups, &mcpAssignSubscriptions{}, nil, nil, nil))
			r := gin.New()
			r.POST("/api/v1/admin/subscriptions/assign", func(c *gin.Context) {
				c.Set("auth_method", method)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
				h.Assign(c)
			})
			call := func(key string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/assign", bytes.NewBufferString(`{"user_id":8,"group_id":4,"validity_days":7}`))
				req.Header.Set("Content-Type", "application/json")
				if key != "" {
					req.Header.Set("Idempotency-Key", key)
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w
			}
			first := call("")
			if method == service.AuditAuthMethodJWT {
				require.Equal(t, http.StatusOK, first.Code, first.Body.String())
				require.Equal(t, 1, groups.reads)
				return
			}
			require.Equal(t, http.StatusBadRequest, first.Code, first.Body.String())
			require.Zero(t, groups.reads)
			created, replayed := call("mcp-assignment"), call("mcp-assignment")
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
			require.JSONEq(t, created.Body.String(), replayed.Body.String())
			require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
			require.Equal(t, 1, groups.reads, "MCP retry must replay before invoking the business service again")
		})
	}
}
