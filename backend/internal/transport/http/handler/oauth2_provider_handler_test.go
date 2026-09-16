package handler

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuth2ClientCredentialsRejectsMultipleAuthenticationMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	form := url.Values{"client_id": {"client-1"}, "client_secret": {"secret-1"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("client-1:secret-1")))
	require.NoError(t, req.ParseForm())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	_, _, err := oauth2ClientCredentials(c)
	require.Error(t, err)
}

func TestOAuth2ClientCredentialsAcceptsBasicOrPost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("basic", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("client-1:secret-1")))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		clientID, clientSecret, err := oauth2ClientCredentials(c)
		require.NoError(t, err)
		require.Equal(t, "client-1", clientID)
		require.Equal(t, "secret-1", clientSecret)
	})

	t.Run("post", func(t *testing.T) {
		form := url.Values{"client_id": {"client-2"}, "client_secret": {"secret-2"}}
		req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.NoError(t, req.ParseForm())
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		clientID, clientSecret, err := oauth2ClientCredentials(c)
		require.NoError(t, err)
		require.Equal(t, "client-2", clientID)
		require.Equal(t, "secret-2", clientSecret)
	})
}

func TestOAuth2BearerTokenRequiresExactlyOneBearerCredential(t *testing.T) {
	token, ok := oauth2BearerToken("Bearer s2a_test")
	require.True(t, ok)
	require.Equal(t, "s2a_test", token)

	for _, header := range []string{"", "Basic s2a_test", "Bearer", "Bearer one two"} {
		_, ok := oauth2BearerToken(header)
		require.False(t, ok, header)
	}
}

func TestOAuth2FormContentTypeRequiresExactMediaType(t *testing.T) {
	require.True(t, isOAuth2FormContentType("application/x-www-form-urlencoded"))
	require.True(t, isOAuth2FormContentType("Application/X-Www-Form-Urlencoded; charset=UTF-8"))
	for _, contentType := range []string{"", "application/json", "application/x-www-form-urlencoded-evil"} {
		require.False(t, isOAuth2FormContentType(contentType), contentType)
	}
}
