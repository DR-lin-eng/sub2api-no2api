package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMediaStudioHandlerRejectsOversizedSessionBodyWithoutCachingCredentialResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/media-studio/session",
		bytes.NewReader(bytes.Repeat([]byte{' '}, maxMediaStudioRequestBodyBytes+1)),
	)
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})

	(&MediaStudioHandler{}).CreateSession(context)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
}

func TestMediaStudioHandlerRejectsOversizedAdminRoutesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/media-studio/group-routes",
		bytes.NewReader(bytes.Repeat([]byte{' '}, maxMediaStudioRequestBodyBytes+1)),
	)
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	(&MediaStudioHandler{}).UpdateAdminGroupRoutes(context)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}
