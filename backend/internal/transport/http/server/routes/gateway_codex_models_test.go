package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesModelsPathsUseDistinctHandlers(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	registered := make(map[string]string)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			registered[route.Path] = route.Handler
		}
	}

	probe := gin.New()
	probe.GET("/local-models", (&handler.GatewayHandler{}).Models)
	probe.GET("/codex-models", (&handler.OpenAIGatewayHandler{}).CodexModels)
	expectedLocalHandler := probe.Routes()[0].Handler
	expectedCodexHandler := probe.Routes()[1].Handler

	require.Equal(t, expectedCodexHandler, registered["/backend-api/codex/models"], "explicit Codex path should use the manifest handler")
	require.Equal(t, expectedLocalHandler, registered["/v1/models"], "GET /v1/models should use the local model-list handler")
	require.Equal(t, expectedLocalHandler, registered["/models"], "root alias should use the local /v1/models handler")
}
