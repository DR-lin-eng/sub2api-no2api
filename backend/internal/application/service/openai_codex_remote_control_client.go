package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/modules/codexremotecontrol"
	"github.com/Wei-Shaw/sub2api/internal/platform/devicekey"
)

type CodexRemoteControlClientOptions struct {
	DesktopHeaders http.Header
	APIBaseURL     string
	ResourcesPath  string
	StateDirectory string
	InstallationID string
	// The caller supplies this account's configured proxy/IPv6/TLS transport.
	HTTPClient    *http.Client
	RequestStepUp codexremotecontrol.StepUp
}

// NewCodexRemoteControlClient binds the controller to the resolved credential
// owner. It constructs a client; only an explicit ConnectSession call can enroll.
func NewCodexRemoteControlClient(ctx context.Context, repository AccountRepository, account *Account, options CodexRemoteControlClientOptions) (*codexremotecontrol.Manager, error) {
	owner, err := resolveCredentialAccount(ctx, repository, account)
	if err != nil {
		return nil, err
	}
	if owner == nil || !owner.IsOpenAIOAuth() || owner.IsOpenAIPersonalAccessToken() || owner.IsOpenAIAgentIdentity() || options.HTTPClient == nil {
		return nil, errors.New("remote control client requires an OAuth owner and account-scoped transport")
	}
	token := owner.GetCredential("access_token")
	if err := validateOpenAIOAuthRequestPrincipal(owner, token); err != nil {
		return nil, err
	}
	auth, err := codexremotecontrol.AuthFromToken(token, owner.GetChatGPTAccountID())
	if err != nil {
		return nil, err
	}
	store, err := codexremotecontrol.NewFileStore(options.StateDirectory, options.APIBaseURL, auth.AccountUserID, options.InstallationID)
	if err != nil {
		return nil, err
	}
	return codexremotecontrol.New(codexremotecontrol.Config{BaseURL: options.APIBaseURL, Auth: auth, Store: store, Keys: devicekey.NewProvider(options.ResourcesPath), HTTPClient: options.HTTPClient, DesktopHeaders: options.DesktopHeaders, RequestStepUp: options.RequestStepUp})
}
