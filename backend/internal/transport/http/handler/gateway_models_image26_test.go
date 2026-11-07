package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelsGPTImage26CatalogAndMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformMultimodal, service.PlatformComposite} {
		for _, mapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mapped=%t", platform, mapped), func(t *testing.T) {
				groupID := int64(266)
				repo := &gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{}}
				if mapped {
					accountPlatform := platform
					if platform == service.PlatformComposite {
						accountPlatform = service.PlatformOpenAI
					}
					repo.byGroup[groupID] = []service.Account{{ID: 1, Platform: accountPlatform, Type: service.AccountTypeAPIKey,
						Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2.6-flare": "gpt-image-2.6-flare"}}}}
				}
				h := newGatewayModelsHandlerForTest(repo)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				group := &service.Group{ID: groupID, Platform: platform}
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: group})
				h.Models(c)
				require.Equal(t, http.StatusOK, rec.Code)
				var response gatewayModelsResponseForTest
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
				ids := modelIDsForTest(response.Data)
				require.Contains(t, ids, "gpt-image-2.6-flare")
				if mapped {
					require.NotContains(t, ids, "gpt-image-2.6-sunburst", "account mapping must remain authoritative")
				} else {
					require.Contains(t, ids, "gpt-image-2.6-sunburst")
				}
				group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-image-2.6-flare"}}
				rec = httptest.NewRecorder()
				c, _ = gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: group})
				h.Models(c)
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
				require.Equal(t, []string{"gpt-image-2.6-flare"}, modelIDsForTest(response.Data), "group allowlist must remain authoritative")
			})
		}
	}
}
