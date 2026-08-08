package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExchangePendingOAuthCompletionChoiceStateDoesNotBindIdentity(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	victim, err := client.User.Create().
		SetEmail("victim@example.com").
		SetUsername("victim-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("choice-state-attack-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("attacker-subject-123").
		SetTargetUserID(victim.ID).
		SetResolvedEmail(victim.Email).
		SetBrowserSessionKey("choice-state-attack-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "attacker_linuxdo_user",
			"suggested_display_name": "Attacker Display Name",
			"suggested_avatar_url":   "https://cdn.example/attacker.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"step":                      oauthPendingChoiceStep,
				"adoption_required":         true,
				"force_email_on_signup":     true,
				"email_binding_required":    true,
				"existing_account_bindable": true,
				"email":                     victim.Email,
				"resolved_email":            victim.Email,
				"redirect":                  "/dashboard",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":true,"adopt_avatar":true}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeJSONResponseData(t, recorder)
	require.NotContains(t, data, "access_token")
	require.Equal(t, oauthPendingChoiceStep, data["step"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("attacker-subject-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	decisionCount, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, decisionCount)

	storedVictim, err := client.User.Get(ctx, victim.ID)
	require.NoError(t, err)
	require.Equal(t, "victim-user", storedVictim.Username)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}
