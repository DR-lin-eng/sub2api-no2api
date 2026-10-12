//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeProbeRejectsModelsForDedicatedEndpoints(t *testing.T) {
	for _, model := range []string{"gemini-3.1-pro", "opencode/jev-latest"} {
		t.Run(model, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
			err := (&AccountTestService{}).testOpenAIAccountConnection(ctx, account, model, "", "")
			require.Error(t, err)
			require.Contains(t, writer.Body.String(), "not supported on OpenCode standard gateway")
		})
	}
}
