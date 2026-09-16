package handler

import (
	"encoding/base64"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

type OAuth2ProviderHandler struct {
	service *service.OAuth2ProviderService
}

func NewOAuth2ProviderHandler(provider *service.OAuth2ProviderService) *OAuth2ProviderHandler {
	return &OAuth2ProviderHandler{service: provider}
}

type OAuth2AuthorizationRequestPayload struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	ResponseType        string `json:"response_type"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

type OAuth2AuthorizePayload struct {
	OAuth2AuthorizationRequestPayload
	Approved bool `json:"approved"`
}

func (p OAuth2AuthorizationRequestPayload) serviceRequest() service.OAuth2AuthorizationRequest {
	return service.OAuth2AuthorizationRequest{
		ClientID:            strings.TrimSpace(p.ClientID),
		RedirectURI:         p.RedirectURI,
		ResponseType:        p.ResponseType,
		Scope:               p.Scope,
		State:               p.State,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
	}
}

func (h *OAuth2ProviderHandler) AuthorizationPreview(c *gin.Context) {
	oauth2NoStore(c)
	if h == nil || h.service == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	preview, err := h.service.ValidateAuthorizationRequest(c.Request.Context(), service.OAuth2AuthorizationRequest{
		ClientID:            strings.TrimSpace(c.Query("client_id")),
		RedirectURI:         c.Query("redirect_uri"),
		ResponseType:        c.Query("response_type"),
		Scope:               c.Query("scope"),
		State:               c.Query("state"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
	})
	if err != nil {
		if writeOAuth2ProtocolError(c, err) {
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}

func (h *OAuth2ProviderHandler) Authorize(c *gin.Context) {
	oauth2NoStore(c)
	if h == nil || h.service == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var payload OAuth2AuthorizePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid OAuth2 authorization request")
		return
	}
	result, err := h.service.Authorize(c.Request.Context(), subject.UserID, payload.serviceRequest(), payload.Approved)
	if err != nil {
		if writeOAuth2ProtocolError(c, err) {
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OAuth2ProviderHandler) Discovery(c *gin.Context) {
	if h == nil || h.service == nil {
		writeOAuth2JSONError(c, http.StatusNotFound, "not_found", "OAuth2 provider is unavailable")
		return
	}
	document, err := h.service.Discovery(c.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrOAuth2ProviderDisabled) {
			writeOAuth2JSONError(c, http.StatusNotFound, "not_found", "OAuth2 provider is disabled")
			return
		}
		writeOAuth2JSONError(c, http.StatusInternalServerError, "server_error", "OAuth2 provider metadata is unavailable")
		return
	}
	writeOAuth2JSON(c, http.StatusOK, document)
}

func (h *OAuth2ProviderHandler) Token(c *gin.Context) {
	if h == nil || h.service == nil {
		writeOAuth2JSONError(c, http.StatusServiceUnavailable, "temporarily_unavailable", "OAuth2 provider is unavailable")
		return
	}
	if !isOAuth2FormContentType(c.GetHeader("Content-Type")) {
		writeOAuth2JSONError(c, http.StatusBadRequest, "invalid_request", "token requests must use application/x-www-form-urlencoded")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.Request.ParseForm(); err != nil {
		writeOAuth2JSONError(c, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	clientID, clientSecret, err := oauth2ClientCredentials(c)
	if err != nil {
		c.Header("WWW-Authenticate", `Basic realm="oauth2/token"`)
		writeOAuth2JSONError(c, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	result, err := h.service.ExchangeToken(c.Request.Context(), service.OAuth2TokenRequest{
		GrantType:    c.PostForm("grant_type"),
		Code:         c.PostForm("code"),
		RedirectURI:  c.PostForm("redirect_uri"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CodeVerifier: c.PostForm("code_verifier"),
	})
	if err != nil {
		writeOAuth2ServiceError(c, err)
		return
	}
	writeOAuth2JSON(c, http.StatusOK, result)
}

func (h *OAuth2ProviderHandler) UserInfo(c *gin.Context) {
	if h == nil || h.service == nil {
		writeOAuth2JSONError(c, http.StatusServiceUnavailable, "temporarily_unavailable", "OAuth2 provider is unavailable")
		return
	}
	token, ok := oauth2BearerToken(c.GetHeader("Authorization"))
	if !ok {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeOAuth2JSONError(c, http.StatusUnauthorized, "invalid_token", "Bearer access token is required")
		return
	}
	userinfo, err := h.service.UserInfo(c.Request.Context(), token)
	if err != nil {
		if protocol, ok := asOAuth2ProtocolError(err); ok {
			c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeOAuth2JSONError(c, protocol.HTTPStatus, protocol.Code, protocol.Description)
			return
		}
		writeOAuth2JSONError(c, http.StatusInternalServerError, "server_error", "unable to load user information")
		return
	}
	writeOAuth2JSON(c, http.StatusOK, userinfo)
}

func (h *OAuth2ProviderHandler) Revoke(c *gin.Context) {
	oauth2NoStore(c)
	if h == nil || h.service == nil {
		writeOAuth2JSONError(c, http.StatusServiceUnavailable, "temporarily_unavailable", "OAuth2 provider is unavailable")
		return
	}
	if !isOAuth2FormContentType(c.GetHeader("Content-Type")) {
		writeOAuth2JSONError(c, http.StatusBadRequest, "invalid_request", "revocation requests must use application/x-www-form-urlencoded")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.Request.ParseForm(); err != nil {
		writeOAuth2JSONError(c, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	clientID, clientSecret, err := oauth2ClientCredentials(c)
	if err != nil {
		writeOAuth2JSONError(c, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	if err := h.service.RevokeToken(c.Request.Context(), c.PostForm("token"), clientID, clientSecret); err != nil {
		writeOAuth2ServiceError(c, err)
		return
	}
	c.Status(http.StatusOK)
}

func oauth2ClientCredentials(c *gin.Context) (string, string, error) {
	formID := strings.TrimSpace(c.PostForm("client_id"))
	formSecret := c.PostForm("client_secret")
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		return formID, formSecret, nil
	}
	if formID != "" || formSecret != "" {
		return "", "", errors.New("multiple client authentication methods")
	}
	if !strings.HasPrefix(strings.ToLower(authHeader), "basic ") {
		return "", "", errors.New("unsupported client authentication scheme")
	}
	encoded := strings.TrimSpace(authHeader[len("Basic "):])
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("malformed basic authentication")
	}
	basicID, basicSecret := parts[0], parts[1]
	return basicID, basicSecret, nil
}

func isOAuth2FormContentType(raw string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(mediaType, "application/x-www-form-urlencoded")
}

func oauth2BearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" || len(parts[1]) > 256 {
		return "", false
	}
	return parts[1], true
}

func asOAuth2ProtocolError(err error) (*service.OAuth2ProtocolError, bool) {
	var protocol *service.OAuth2ProtocolError
	if errors.As(err, &protocol) {
		return protocol, true
	}
	return nil, false
}

func writeOAuth2ServiceError(c *gin.Context, err error) {
	if protocol, ok := asOAuth2ProtocolError(err); ok {
		if protocol.Code == "invalid_client" {
			c.Header("WWW-Authenticate", `Basic realm="oauth2/token"`)
		}
		if protocol.Headers != nil {
			for key, value := range protocol.Headers {
				c.Header(key, value)
			}
		}
		writeOAuth2JSONError(c, protocol.HTTPStatus, protocol.Code, protocol.Description)
		return
	}
	writeOAuth2JSONError(c, http.StatusInternalServerError, "server_error", "OAuth2 provider request failed")
}

func writeOAuth2ProtocolError(c *gin.Context, err error) bool {
	protocol, ok := asOAuth2ProtocolError(err)
	if !ok {
		return false
	}
	writeOAuth2JSONError(c, protocol.HTTPStatus, protocol.Code, protocol.Description)
	return true
}

func writeOAuth2JSON(c *gin.Context, status int, payload any) {
	oauth2NoStore(c)
	c.JSON(status, payload)
}

func oauth2NoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func writeOAuth2JSONError(c *gin.Context, status int, code, description string) {
	writeOAuth2JSON(c, status, gin.H{"error": code, "error_description": description})
}
