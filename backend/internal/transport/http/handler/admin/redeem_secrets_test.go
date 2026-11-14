package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminVoucherSecretsIndependentPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, actor := range []struct {
		name   string
		scopes []string
		jwt    bool
		secret bool
	}{
		{name: "read", scopes: []string{service.AdminAPIKeyScopeRead}},
		{name: "write does not imply secrets", scopes: []string{service.AdminAPIKeyScopeWrite}},
		{name: "dedicated permission", scopes: []string{service.AdminAPIKeyScopeRead, service.AdminAPIKeyScopeRedeemSecrets}, secret: true},
		{name: "human admin", jwt: true, secret: true},
	} {
		t.Run(actor.name, func(t *testing.T) {
			stub := newStubAdminService()
			redeem := NewRedeemHandler(stub, nil)
			user := &UserHandler{adminService: stub}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if !actor.jwt {
					c.Set("auth_method", "admin_api_key")
					c.Set("admin_api_key_scopes", actor.scopes)
				}
			})
			router.GET("/codes", redeem.List)
			router.GET("/codes/:id", redeem.GetByID)
			router.GET("/users/:id/history", user.GetBalanceHistory)
			router.GET("/export", redeem.Export)
			router.POST("/export-generated", redeem.ExportGenerated)
			router.POST("/generate", redeem.Generate)
			router.POST("/create-and-redeem", redeem.CreateAndRedeem)
			for _, path := range []string{"/codes", "/codes/5", "/users/1/history"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				require.Equal(t, http.StatusOK, response.Code)
				if actor.secret {
					require.Contains(t, response.Body.String(), "R-TEST")
				} else {
					require.NotContains(t, response.Body.String(), "R-TEST")
					require.Contains(t, response.Body.String(), "********")
				}
			}
			if !actor.secret {
				before := stub.lastListRedeemCodes.calls
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/codes?search=R-T", nil))
				require.Equal(t, http.StatusForbidden, response.Code)
				require.Equal(t, before, stub.lastListRedeemCodes.calls, "no substring-count oracle")
				for _, request := range []struct{ method, path, body string }{
					{http.MethodGet, "/export", ""}, {http.MethodPost, "/export-generated", `{"ids":[5],"format":"txt"}`},
					{http.MethodPost, "/generate", `{"count":1,"type":"balance","value":10}`},
					{http.MethodPost, "/create-and-redeem", `{"code":"fixture","value":10,"user_id":1}`},
				} {
					response := httptest.NewRecorder()
					req := httptest.NewRequest(request.method, request.path, bytes.NewBufferString(request.body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", "previous-privileged-request")
					router.ServeHTTP(response, req)
					require.Equal(t, http.StatusForbidden, response.Code)
				}
			} else {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/export", nil))
				require.Equal(t, http.StatusOK, response.Code)
				require.Contains(t, response.Body.String(), "R-TEST")
			}
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			if !actor.jwt {
				context.Set("auth_method", "admin_api_key")
				context.Set("admin_api_key_scopes", actor.scopes)
			}
			order := redactPaymentVoucher(context, &AdminPaymentOrderResult{RechargeCode: "PAY-VOUCHER"})
			if actor.secret {
				require.Equal(t, "PAY-VOUCHER", order.RechargeCode)
			} else {
				require.Empty(t, order.RechargeCode)
			}
		})
	}
}

func TestRevokedVoucherScopeCannotReplayPrivilegedGeneratedCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	scopes := []string{service.AdminAPIKeyScopeWrite, service.AdminAPIKeyScopeRedeemSecrets}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("auth_method", "admin_api_key"); c.Set("admin_api_key_scopes", scopes) })
	router.POST("/generate", NewRedeemHandler(newStubAdminService(), nil).Generate)
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"count":1,"type":"balance","value":10}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "cached-voucher-fixture")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	first := request()
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), "R-TEST")
	scopes = []string{service.AdminAPIKeyScopeWrite}
	replay := request()
	require.Equal(t, http.StatusForbidden, replay.Code)
	require.NotContains(t, replay.Body.String(), "R-TEST")
}
