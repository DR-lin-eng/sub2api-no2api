package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AuthSubject is the minimal authenticated identity stored in gin context.
// Decision: {UserID int64, Concurrency int, SchedulingTier service.RequestSchedulingTier}
type AuthSubject struct {
	UserID         int64
	Concurrency    int
	SchedulingTier service.RequestSchedulingTier
}

// setAuthSubject is the single production construction path for AuthSubject.
// Keeping the tier explicit avoids treating an omitted zero value as priority.
func setAuthSubject(c *gin.Context, userID int64, concurrency int, tier service.RequestSchedulingTier) {
	tier = service.NormalizeRequestSchedulingTier(tier)
	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:         userID,
		Concurrency:    concurrency,
		SchedulingTier: tier,
	})
}

func GetAuthSubjectFromContext(c *gin.Context) (AuthSubject, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return AuthSubject{}, false
	}
	subject, ok := value.(AuthSubject)
	return subject, ok
}

func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyUserRole))
	if !exists {
		return "", false
	}
	role, ok := value.(string)
	return role, ok
}
