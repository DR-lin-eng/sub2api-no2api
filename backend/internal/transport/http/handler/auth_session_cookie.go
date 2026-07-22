package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const refreshTokenCookieName = "sub2api_refresh_token"

// setRefreshTokenCookie keeps the long-lived refresh credential out of
// JavaScript-readable storage. The access token may remain in memory for the
// current page, while the browser sends this HttpOnly cookie only to auth APIs.
func setRefreshTokenCookie(c *gin.Context, token string, maxAge time.Duration) {
	if c == nil || strings.TrimSpace(token) == "" {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    strings.TrimSpace(token),
		Path:     "/api/v1/auth",
		MaxAge:   maxAgeSeconds(maxAge),
		Expires:  time.Now().Add(maxAge),
		HttpOnly: true,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func readRefreshTokenCookie(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	cookie, err := c.Request.Cookie(refreshTokenCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func clearRefreshTokenCookie(c *gin.Context) {
	if c == nil {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func maxAgeSeconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	seconds := int(value / time.Second)
	if seconds <= 0 {
		return 1
	}
	return seconds
}
