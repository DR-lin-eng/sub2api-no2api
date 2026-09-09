package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/stretchr/testify/require"
)

func TestAdminAccountCodexPrewarmForce(t *testing.T) {
	ctx := context.Background()
	settings := NewSettingService(newCodexSimulationSettingRepo(), nil)
	t.Cleanup(func() { codexsimulation.SetPrewarmContinuationEnabled(false) })
	_, err := settings.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		CodexPrewarmContinuationForceEnabled: true,
		ContinuationMode:                     "off",
		StateTTLSeconds:                      60,
	})
	require.NoError(t, err)

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		t.Run(accountType, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{}
			svc := &adminServiceImpl{accountRepo: repo, settingService: settings}
			inputExtra := map[string]any{CodexPrewarmContinuationExtraKey: false, "custom": "preserved"}
			created, err := svc.CreateAccount(ctx, &CreateAccountInput{
				Name: "prewarm-test", Platform: PlatformOpenAI, Type: accountType,
				Extra: inputExtra, SkipDefaultGroupBind: true,
			})
			require.NoError(t, err)
			want := accountType == AccountTypeOAuth
			require.Equal(t, want, created.Extra[CodexPrewarmContinuationExtraKey])
			require.Equal(t, false, inputExtra[CodexPrewarmContinuationExtraKey])
			require.Equal(t, "preserved", created.Extra["custom"])

			updated, err := svc.UpdateAccount(ctx, created.ID, &UpdateAccountInput{Extra: inputExtra})
			require.NoError(t, err)
			require.Equal(t, want, updated.Extra[CodexPrewarmContinuationExtraKey])
			require.Equal(t, "preserved", updated.Extra["custom"])

			// Empty patches must retain their original no-op behavior when the
			// global switch is on, including a nil map from internal callers.
			require.NotPanics(t, func() {
				require.NoError(t, svc.UpdateAccountExtra(ctx, created.ID, nil))
			})
			require.Empty(t, repo.updates)
			require.NoError(t, svc.UpdateAccountExtra(ctx, created.ID, map[string]any{}))
			require.Empty(t, repo.updates)

			require.NoError(t, svc.UpdateAccountExtra(ctx, created.ID, map[string]any{
				CodexPrewarmContinuationExtraKey: false, "custom": "updated",
			}))
			persisted, err := repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, want, persisted.Extra[CodexPrewarmContinuationExtraKey])
			require.Equal(t, "updated", persisted.Extra["custom"])
		})
	}
}
