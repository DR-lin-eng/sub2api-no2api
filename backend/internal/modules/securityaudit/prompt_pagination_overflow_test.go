package securityaudit

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPromptEventsRejectOverflowBeforeQuery(t *testing.T) {
	called := false
	svc := &fakePromptAdminService{list: func(context.Context, EventFilter, int, int) (*EventPage, error) {
		called = true
		return &EventPage{}, nil
	}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/?page=%d&page_size=100", math.MaxInt), nil)
	(&PromptAdminHandler{service: svc}).ListEvents(c)
	require.Equal(t, 400, rec.Code)
	require.False(t, called)
}
