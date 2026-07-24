package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

func TestSetAuthSubjectStoresExplicitSchedulingTierWithoutRequestAllocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)

	setAuthSubject(c, 42, 7, service.RequestSchedulingTierLow)

	subject, ok := GetAuthSubjectFromContext(c)
	if !ok || subject.UserID != 42 || subject.Concurrency != 7 || subject.SchedulingTier != service.RequestSchedulingTierLow {
		t.Fatalf("unexpected auth subject: %#v, %v", subject, ok)
	}
	if tier, explicit := service.RequestSchedulingTierFromContextOK(c.Request.Context()); explicit {
		t.Fatalf("auth middleware must not allocate a tier context before admission is enabled, got %d", tier)
	}
}

func TestSetAuthSubjectNormalizesInvalidTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)

	setAuthSubject(c, 1, 1, service.RequestSchedulingTier(99))

	subject, _ := GetAuthSubjectFromContext(c)
	if subject.SchedulingTier != service.RequestSchedulingTierNormal {
		t.Fatalf("expected invalid tier to normalize to normal, got %d", subject.SchedulingTier)
	}
}
