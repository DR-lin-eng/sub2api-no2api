package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/chat"
	middleware2 "github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminChatHandlerSendMessageRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"content":"` + strings.Repeat("a", maxChatRequestBodyBytes) + `"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/conversations/1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})

	NewChatHandler(nil, chat.NewHub()).SendMessage(c)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Request body too large")
}

func TestLimitChatSearchPreservesUnicodeCharacters(t *testing.T) {
	search := limitChatSearch("  " + strings.Repeat("你", maxChatSearchRunes+1) + "  ")

	require.True(t, utf8.ValidString(search))
	require.Equal(t, maxChatSearchRunes, utf8.RuneCountInString(search))
}
