package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSIWCGrantValidation(t *testing.T) {
	valid := TokenResponse{AccessToken: "at", RefreshToken: "rt", IDToken: "id", ExpiresIn: 3600, Scope: SIWCScope}
	require.NoError(t, ValidateSIWCTokenResponse(&valid, true))
	require.Error(t, ValidateSIWCTokenResponse(nil, true))
	missingID := valid
	missingID.IDToken = ""
	require.Error(t, ValidateSIWCTokenResponse(&missingID, true))
	require.NoError(t, ValidateSIWCTokenResponse(&missingID, false))
	missingScope := valid
	missingScope.Scope = "openid " + SIWCDirectTokenScope + ".invalid"
	require.Error(t, ValidateSIWCTokenResponse(&missingScope, true))
}

func TestSIWCAudienceSupportsArrayAndString(t *testing.T) {
	for _, audience := range []string{`"oaiapp_issued"`, `["oaiapp_issued"]`} {
		var claims IDTokenClaims
		err := json.Unmarshal([]byte(`{"aud":`+audience+`,"nonce":"nonce","email":"user@example.com"}`), &claims)
		require.NoError(t, err)
		require.Equal(t, []string{"oaiapp_issued"}, claims.Aud)
		require.Equal(t, "nonce", claims.Nonce)
		require.Equal(t, "user@example.com", claims.Email)
	}
}
