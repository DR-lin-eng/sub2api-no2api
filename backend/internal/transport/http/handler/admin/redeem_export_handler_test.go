package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupRedeemExportRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewRedeemHandler(adminSvc, nil)
	router.GET("/api/v1/admin/redeem-codes/export", h.Export)
	router.POST("/api/v1/admin/redeem-codes/export-generated", h.ExportGenerated)
	router.POST("/api/v1/admin/redeem-codes/generate", h.Generate)
	return router, adminSvc
}

func postRedeemJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestRedeemExportPassesSearchAndSort(t *testing.T) {
	router, adminSvc := setupRedeemExportRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/redeem-codes/export?type=balance&status=unused&search=ABC&sort_by=value&sort_order=asc", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListRedeemCodes.calls)
	require.Equal(t, "balance", adminSvc.lastListRedeemCodes.codeType)
	require.Equal(t, "unused", adminSvc.lastListRedeemCodes.status)
	require.Equal(t, "ABC", adminSvc.lastListRedeemCodes.search)
	require.Equal(t, "value", adminSvc.lastListRedeemCodes.sortBy)
	require.Equal(t, "asc", adminSvc.lastListRedeemCodes.sortOrder)
}

func TestRedeemExportSortDefaults(t *testing.T) {
	router, adminSvc := setupRedeemExportRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/redeem-codes/export", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListRedeemCodes.calls)
	require.Equal(t, "id", adminSvc.lastListRedeemCodes.sortBy)
	require.Equal(t, "desc", adminSvc.lastListRedeemCodes.sortOrder)
}

func TestRedeemGenerateAcceptsMoreThanFormerLimit(t *testing.T) {
	router, _ := setupRedeemExportRouter()

	rec := postRedeemJSON(t, router, "/api/v1/admin/redeem-codes/generate", map[string]any{
		"count": 101,
		"type":  "balance",
		"value": 10,
	})

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRedeemExportGeneratedCSV(t *testing.T) {
	router, adminSvc := setupRedeemExportRouter()
	adminSvc.redeems = []service.RedeemCode{
		{ID: 11, Code: "NEW-ONE"},
		{ID: 12, Code: "NEW-TWO"},
	}

	rec := postRedeemJSON(t, router, "/api/v1/admin/redeem-codes/export-generated", map[string]any{
		"ids":    []int64{12, 11},
		"format": "csv",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), ".csv")
	require.Equal(t, "code\nNEW-TWO\nNEW-ONE\n", rec.Body.String())
}

func TestRedeemExportGeneratedTXT(t *testing.T) {
	router, adminSvc := setupRedeemExportRouter()
	adminSvc.redeems = []service.RedeemCode{{ID: 21, Code: "NEW-TEXT"}}

	rec := postRedeemJSON(t, router, "/api/v1/admin/redeem-codes/export-generated", map[string]any{
		"ids":    []int64{21},
		"format": "txt",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Disposition"), ".txt")
	require.Equal(t, "NEW-TEXT\n", rec.Body.String())
}

func TestRedeemExportGeneratedRejectsUnknownFormat(t *testing.T) {
	router, _ := setupRedeemExportRouter()
	rec := postRedeemJSON(t, router, "/api/v1/admin/redeem-codes/export-generated", map[string]any{
		"ids":    []int64{1},
		"format": "json",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
