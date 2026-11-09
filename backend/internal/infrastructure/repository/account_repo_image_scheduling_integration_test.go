//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestImageIndependentSchedulingCandidateQueries(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		name := "ungrouped"
		if grouped {
			name = "grouped"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			tx := testEntTx(t)
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			group := mustCreateGroup(t, client, &service.Group{Name: "image-scheduling", Platform: service.PlatformOpenAI})
			future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
			normal := mustCreateAccount(t, client, &service.Account{Name: "image-normal", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey})
			paused := mustCreateAccount(t, client, &service.Account{Name: "image-paused", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey})
			_, err := client.Account.UpdateOneID(paused.ID).SetSchedulable(false).Save(ctx)
			require.NoError(t, err)
			ids := []int64{normal.ID, paused.ID}
			for _, state := range []string{"error", "disabled", "cooldown", "limited", "overload", "expired", "other-platform", "deleted"} {
				a := mustCreateAccount(t, client, &service.Account{Name: "image-" + state, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey})
				update := client.Account.UpdateOneID(a.ID)
				switch state {
				case "error":
					update.SetStatus(service.StatusError)
				case "disabled":
					update.SetStatus(service.StatusDisabled)
				case "cooldown":
					update.SetTempUnschedulableUntil(future)
				case "limited":
					update.SetRateLimitResetAt(future)
				case "overload":
					update.SetOverloadUntil(future)
				case "expired":
					update.SetExpiresAt(past).SetAutoPauseOnExpired(true)
				case "other-platform":
					update.SetPlatform(service.PlatformGemini)
				case "deleted":
					update.SetDeletedAt(past)
				}
				_, err := update.Save(ctx)
				require.NoError(t, err)
				ids = append(ids, a.ID)
			}
			if grouped {
				for _, id := range ids {
					_, err := client.AccountGroup.Create().SetGroupID(group.ID).SetAccountID(id).Save(ctx)
					require.NoError(t, err)
				}
			}
			type query func(context.Context) ([]service.Account, error)
			queries := map[string]query{
				"platform": func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableByPlatform(ctx, service.PlatformOpenAI)
				},
				"platforms": func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableByPlatforms(ctx, []string{service.PlatformOpenAI})
				},
			}
			if grouped {
				queries["group/platform"] = func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableByGroupIDAndPlatform(ctx, group.ID, service.PlatformOpenAI)
				}
				queries["group/platforms"] = func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableByGroupIDAndPlatforms(ctx, group.ID, []string{service.PlatformOpenAI})
				}
			} else {
				queries["ungrouped/platform"] = func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableUngroupedByPlatform(ctx, service.PlatformOpenAI)
				}
				queries["ungrouped/platforms"] = func(ctx context.Context) ([]service.Account, error) {
					return repo.ListSchedulableUngroupedByPlatforms(ctx, []string{service.PlatformOpenAI})
				}
			}
			for label, query := range queries {
				t.Run(label, func(t *testing.T) {
					for _, independent := range []bool{false, true} {
						requestCtx := ctx
						want := []int64{normal.ID}
						if independent {
							requestCtx = service.WithImageIndependentScheduling(ctx)
							want = append(want, paused.ID)
						}
						accounts, err := query(requestCtx)
						require.NoError(t, err)
						got := make([]int64, 0)
						for _, a := range accounts {
							for _, id := range ids {
								if a.ID == id {
									got = append(got, id)
								}
							}
						}
						require.ElementsMatch(t, want, got, "independent=%v", independent)
					}
				})
			}
		})
	}
}
