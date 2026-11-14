package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type siwcHandlerClient struct{ nonce string }

func (*siwcHandlerClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	return nil, fmt.Errorf("legacy endpoint must not be used")
}
func (*siwcHandlerClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, fmt.Errorf("legacy endpoint must not be used")
}
func (*siwcHandlerClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, fmt.Errorf("legacy endpoint must not be used")
}
func (c *siwcHandlerClient) ExchangeSIWCCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"nonce":%q,"aud":"oaiapp_issued","email":"siwc@example.com"}`, c.nonce)))
	return &openai.TokenResponse{AccessToken: "at", RefreshToken: "rt", IDToken: "e30." + payload + ".signature", Scope: openai.SIWCScope, ExpiresIn: 3600}, nil
}
func (*siwcHandlerClient) RefreshSIWCToken(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, fmt.Errorf("not used")
}

func TestSIWCAdminGenerateAndExchangeHTTP(t *testing.T) {
	client := &siwcHandlerClient{}
	svc := service.NewOpenAIOAuthService(nil, client)
	defer svc.Stop()
	handler := NewOpenAIOAuthHandler(svc, nil, nil, nil)
	router := gin.New()
	router.POST("/generate", handler.GenerateAuthURL)
	router.POST("/exchange", handler.ExchangeCode)
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}
	rec := post("/generate", `{"flow":"siwc"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var generated struct {
		Data service.OpenAIAuthURLResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &generated))
	authURL, err := url.Parse(generated.Data.AuthURL)
	require.NoError(t, err)
	client.nonce = authURL.Query().Get("nonce")
	callback := openai.SIWCRedirectURI + "?" + url.Values{"code": {"code"}, "state": {authURL.Query().Get("state")}, "client_id": {"oaiapp_issued"}}.Encode()
	body, err := json.Marshal(map[string]string{"session_id": generated.Data.SessionID, "code": callback})
	require.NoError(t, err)
	rec = post("/exchange", string(body))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var exchanged struct {
		Data service.OpenAITokenInfo `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exchanged))
	require.Equal(t, "siwc", exchanged.Data.AuthMode)
	require.Equal(t, "oaiapp_issued", exchanged.Data.ClientID)
	require.NotEmpty(t, exchanged.Data.DeviceID)
	require.Equal(t, http.StatusBadRequest, post("/exchange", string(body)).Code)
	require.Equal(t, http.StatusBadRequest, post("/generate", `{"flow":"unknown"}`).Code)
	require.Equal(t, http.StatusBadRequest, post("/generate", `{"flow":123}`).Code)
	rec = post("/generate", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "/oauth/authorize?")
}
