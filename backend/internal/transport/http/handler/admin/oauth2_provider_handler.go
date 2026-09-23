package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// OAuth2ProviderHandler exposes the administrator-only OAuth2 provider control plane.
type OAuth2ProviderHandler struct {
	provider *service.OAuth2ProviderService
}

func NewOAuth2ProviderHandler(provider *service.OAuth2ProviderService) *OAuth2ProviderHandler {
	return &OAuth2ProviderHandler{provider: provider}
}

type oauth2ProviderConfigRequest struct {
	Enabled               bool   `json:"enabled"`
	Issuer                string `json:"issuer"`
	AccessTokenTTLSeconds int    `json:"access_token_ttl_seconds"`
}

type oauth2ProviderClientRequest struct {
	Name          string   `json:"name"`
	ClientType    string   `json:"client_type"`
	RedirectURIs  []string `json:"redirect_uris"`
	AllowedScopes []string `json:"allowed_scopes"`
	Enabled       *bool    `json:"enabled"`
}

type oauth2ProviderClientUpdateRequest struct {
	Name          *string   `json:"name"`
	RedirectURIs  *[]string `json:"redirect_uris"`
	AllowedScopes *[]string `json:"allowed_scopes"`
	Enabled       *bool     `json:"enabled"`
}

func (h *OAuth2ProviderHandler) GetConfig(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	config, err := h.provider.GetAdminConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *OAuth2ProviderHandler) UpdateConfig(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	var request oauth2ProviderConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid OAuth2 provider configuration")
		return
	}
	config, err := h.provider.UpdateAdminConfig(c.Request.Context(), service.OAuth2ProviderConfigUpdate{
		Enabled:               request.Enabled,
		Issuer:                request.Issuer,
		AccessTokenTTLSeconds: request.AccessTokenTTLSeconds,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *OAuth2ProviderHandler) CreateClient(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	var request oauth2ProviderClientRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid OAuth2 client configuration")
		return
	}
	result, err := h.provider.CreateClient(c.Request.Context(), service.OAuth2ClientCreateInput{
		Name:          request.Name,
		ClientType:    request.ClientType,
		RedirectURIs:  request.RedirectURIs,
		AllowedScopes: request.AllowedScopes,
		Enabled:       request.Enabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, result)
}

func (h *OAuth2ProviderHandler) UpdateClient(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	var request oauth2ProviderClientUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid OAuth2 client configuration")
		return
	}
	client, err := h.provider.UpdateClient(c.Request.Context(), c.Param("client_id"), service.OAuth2ClientUpdateInput{
		Name:          request.Name,
		RedirectURIs:  request.RedirectURIs,
		AllowedScopes: request.AllowedScopes,
		Enabled:       request.Enabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, client)
}

func (h *OAuth2ProviderHandler) RotateSecret(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	result, err := h.provider.RotateClientSecret(c.Request.Context(), c.Param("client_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OAuth2ProviderHandler) DeleteClient(c *gin.Context) {
	oauth2ProviderAdminNoStore(c)
	if h == nil || h.provider == nil {
		response.InternalError(c, "OAuth2 provider is unavailable")
		return
	}
	if err := h.provider.DeleteClient(c.Request.Context(), c.Param("client_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "OAuth2 client deleted"})
}

func oauth2ProviderAdminNoStore(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Pragma", "no-cache")
}
