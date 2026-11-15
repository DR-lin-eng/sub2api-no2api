package handler

import (
	"net/http"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/ip"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type oauthStartCaptchaRequest struct {
	CaptchaToken          string `json:"captcha_token"`
	TurnstileToken        string `json:"turnstile_token"`
	TencentCaptchaTicket  string `json:"tencent_captcha_ticket"`
	TencentCaptchaRandstr string `json:"tencent_captcha_randstr"`
}

type oauthStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
}

func (h *AuthHandler) requireActionCaptchaForOAuthLoginStart(c *gin.Context) bool {
	if strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/bind/start") {
		return true
	}
	var req oauthStartCaptchaRequest
	if c.Request.Method == http.MethodPost {
		_ = c.ShouldBindJSON(&req)
	}
	var authService *service.AuthService
	var settingService *service.SettingService
	if h != nil {
		authService = h.authService
		settingService = h.settingSvc
	}
	if err := verifyActionCaptcha(
		c.Request.Context(),
		authService,
		settingService,
		humanVerificationProof(req.CaptchaToken, req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr),
		ip.GetClientIP(c),
	); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	return true
}

func respondOAuthStart(c *gin.Context, authorizeURL string) {
	if c.Request.Method == http.MethodPost {
		response.Success(c, oauthStartResponse{AuthorizeURL: authorizeURL})
		return
	}
	c.Redirect(http.StatusFound, authorizeURL)
}
