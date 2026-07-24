package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ops429PassthroughRepo struct {
	rules []*model.ErrorPassthroughRule
}

func (r *ops429PassthroughRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func (*ops429PassthroughRepo) GetByID(context.Context, int64) (*model.ErrorPassthroughRule, error) {
	return nil, nil
}

func (*ops429PassthroughRepo) Create(context.Context, *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return nil, nil
}

func (*ops429PassthroughRepo) Update(context.Context, *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return nil, nil
}

func (*ops429PassthroughRepo) Delete(context.Context, int64) error { return nil }

func newOps429PassthroughService(platform string) *service.ErrorPassthroughService {
	return service.NewErrorPassthroughService(&ops429PassthroughRepo{rules: []*model.ErrorPassthroughRule{{
		Name:            "retain upstream 429",
		Enabled:         true,
		ErrorCodes:      []int{http.StatusTooManyRequests},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{platform},
		PassthroughCode: true,
		PassthroughBody: true,
	}}}, nil)
}

func TestUpstream429PassthroughRulesRetainUpstreamContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"error":{"message":"provider quota exceeded"}}`)

	tests := []struct {
		name string
		run  func(*gin.Context)
	}{
		{
			name: "anthropic",
			run: func(c *gin.Context) {
				h := &GatewayHandler{errorPassthroughService: newOps429PassthroughService(service.PlatformAnthropic)}
				h.handleFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: body}, service.PlatformAnthropic, false)
			},
		},
		{
			name: "openai",
			run: func(c *gin.Context) {
				h := &OpenAIGatewayHandler{errorPassthroughService: newOps429PassthroughService(service.PlatformOpenAI)}
				h.handleFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: body}, false)
			},
		},
		{
			name: "gemini",
			run: func(c *gin.Context) {
				h := &GatewayHandler{errorPassthroughService: newOps429PassthroughService(service.PlatformGemini)}
				h.handleGeminiFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: body})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)

			tt.run(c)

			require.Equal(t, http.StatusTooManyRequests, rec.Code)
			require.True(t, hasOpsUpstreamErrorContext(c))
			require.False(t, service.HasOpsClientBusinessLimited(c))
		})
	}
}
