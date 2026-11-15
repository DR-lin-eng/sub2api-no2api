package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/chat"
	middleware2 "github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatHandlerSendMessageRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"content":"` + strings.Repeat("a", maxChatRequestBodyBytes) + `"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})

	NewChatHandler(nil, chat.NewHub()).SendMessage(c)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Request body too large")
}
