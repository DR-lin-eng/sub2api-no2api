package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

func setOpenAIChatGPTAccountHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	deleteOpenAIHeaderEqualFold(headers, "chatgpt-account-id")
	deleteOpenAIHeaderEqualFold(headers, "x-openai-fedramp")
	if chatgptAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); chatgptAccountID != "" {
		headers.Set("chatgpt-account-id", chatgptAccountID)
	}
	if account.IsChatGPTAccountFedRAMP() {
		headers.Set("x-openai-fedramp", "true")
	} else {
		headers.Del("x-openai-fedramp")
	}
}

// resolveAndSetOpenAIChatGPTAccountHeaders 解析 spark 影子账号至其母账号（凭据透传），
// 再调用 setOpenAIChatGPTAccountHeaders 写入 chatgpt-account-id / x-openai-fedramp 头。
// 普通账号（非影子）为直通，行为与直接调用 setOpenAIChatGPTAccountHeaders 一致。
func resolveAndSetOpenAIChatGPTAccountHeaders(ctx context.Context, repo AccountRepository, headers http.Header, account *Account) error {
	credAccount, err := resolveCredentialAccount(ctx, repo, account)
	if err != nil {
		return err
	}
	// Validate against the credential owner, including Spark shadow accounts.
	// This boundary also applies when workspace discovery is off or a trusted
	// relay supplies the endpoint. Never include either principal in the error.
	if headers != nil {
		if scheme, token, ok := strings.Cut(headers.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
			if err := validateOpenAIOAuthRequestPrincipal(credAccount, token); err != nil {
				return err
			}
		}
	}
	setOpenAIChatGPTAccountHeaders(headers, credAccount)
	return nil
}

// JWT claims are used only to reject a known contradictory account binding,
// never to authenticate or grant access. Opaque tokens and tokens without an
// account claim retain the existing upstream validation path.
func validateOpenAIOAuthRequestPrincipal(account *Account, accessToken string) error {
	if account == nil || !account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() || account.IsOpenAIAgentIdentity() {
		return nil
	}
	_, accountChanged := openAIWorkspaceRoutingAccountID(account, accessToken)
	userChanged := false
	if claims, err := openai.DecodeIDToken(strings.TrimSpace(accessToken)); err == nil && claims.OpenAIAuth != nil {
		storedUser := strings.TrimSpace(account.GetCredential("chatgpt_user_id"))
		tokenUser := strings.TrimSpace(claims.OpenAIAuth.UserID)
		if tokenUser == "" {
			tokenUser = strings.TrimSpace(claims.OpenAIAuth.ChatGPTUserID)
		}
		userChanged = storedUser != "" && tokenUser != "" && storedUser != tokenUser
	}
	if !accountChanged && !userChanged {
		return nil
	}
	return newOpenAIOAuthPrincipalChangedError()
}

// A refresh may persist a newly selected workspace for subsequent requests,
// but it must not replay an existing turn under a different known principal.
func validateOpenAIOAuthRefreshPrincipal(expected, refreshed *Account) error {
	if expected == nil || refreshed == nil || expected.ID != refreshed.ID {
		return newOpenAIOAuthPrincipalChangedError()
	}
	expectedAccount, currentAccount := strings.TrimSpace(expected.GetChatGPTAccountID()), strings.TrimSpace(refreshed.GetChatGPTAccountID())
	expectedUser, currentUser := strings.TrimSpace(expected.GetChatGPTUserID()), strings.TrimSpace(refreshed.GetChatGPTUserID())
	if (expectedAccount != "" && currentAccount != "" && !strings.EqualFold(expectedAccount, currentAccount)) ||
		(expectedUser != "" && currentUser != "" && expectedUser != currentUser) {
		return newOpenAIOAuthPrincipalChangedError()
	}
	return validateOpenAIOAuthRequestPrincipal(expected, refreshed.GetOpenAIAccessToken())
}

func isOpenAIOAuthPrincipalChangedError(err error) bool {
	var failover *UpstreamFailoverError
	return errors.As(err, &failover) && failover != nil && failover.Reason == "openai_oauth_account_changed"
}

func newOpenAIOAuthPrincipalChangedError() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:        http.StatusConflict,
		Scope:             GatewayFailureScopeAccount,
		Reason:            GatewayFailureReason("openai_oauth_account_changed"),
		NextAccountAction: NextAccountRetry,
		ClientStatusCode:  http.StatusConflict,
		ClientMessage:     "The selected OpenAI OAuth account no longer matches its access token",
		ResponseBody:      []byte(`{"error":{"code":"account_changed","type":"authentication_error","message":"The selected OpenAI OAuth account no longer matches its access token"}}`),
	}
}
