package repository

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

func TestSIWCTokenHTTPExchangeAndRefresh(t *testing.T) {
	var forms []url.Values
	server := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/accounts/oauth/token", r.URL.Path)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.NoError(t, r.ParseForm())
		forms = append(forms, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"at","refresh_token":"rt","id_token":"id","scope":%q,"expires_in":3600}`, openai.SIWCScope)
	}))
	defer server.Close()
	client := &openaiOAuthService{tokenURL: server.URL + "/legacy-must-not-be-used", siwcTokenURL: server.URL + "/api/accounts/oauth/token"}
	_, err := client.ExchangeSIWCCode(context.Background(), "code", "verifier", openai.SIWCRedirectURI, "", "oaiapp_issued")
	require.NoError(t, err)
	_, err = client.RefreshSIWCToken(context.Background(), "rt", "", "oaiapp_issued")
	require.NoError(t, err)
	require.Len(t, forms, 2)
	require.Equal(t, "authorization_code", forms[0].Get("grant_type"))
	require.Equal(t, "code", forms[0].Get("code"))
	require.Equal(t, "verifier", forms[0].Get("code_verifier"))
	require.Equal(t, openai.SIWCRedirectURI, forms[0].Get("redirect_uri"))
	require.Equal(t, "refresh_token", forms[1].Get("grant_type"))
	for _, form := range forms {
		require.Equal(t, "oaiapp_issued", form.Get("client_id"))
		require.Equal(t, openai.SIWCResource, form.Get("resource"))
		require.Empty(t, form.Get("scope"))
	}
}

func TestSIWCRefreshPreservesPermanentErrorCodeWithoutTokenBody(t *testing.T) {
	server := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"code":"invalid_grant","message":"sensitive-token-body"}}`)
	}))
	defer server.Close()
	client := &openaiOAuthService{siwcTokenURL: server.URL}
	_, err := client.RefreshSIWCToken(context.Background(), "rt", "", "oaiapp_issued")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_grant")
	require.NotContains(t, err.Error(), "sensitive-token-body")
}
