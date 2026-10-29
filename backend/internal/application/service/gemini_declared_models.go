package service

import (
	"context"
	"sort"
	"strings"
)

type geminiMetadataAccountsKey struct{}

type geminiMetadataAccounts struct {
	groupID    int64
	hasGroupID bool
	accounts   []Account
}

// PrepareGeminiModelsMetadata returns the public mapping keys when every Gemini
// account restricts its models. An unrestricted account (including OAuth
// without an explicit mapping) keeps the existing upstream metadata path.
// Only metadata endpoints call this method; inference scheduling is unchanged.
// The returned context lets that fallback reuse this request's account lookup.
func (s *GeminiMessagesCompatService) PrepareGeminiModelsMetadata(ctx context.Context, groupID *int64) (context.Context, []string, error) {
	if s == nil || (s.accountRepo == nil && s.schedulerSnapshot == nil) {
		return ctx, nil, nil
	}
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformGemini, false)
	if err != nil {
		return ctx, nil, err
	}
	snapshot := &geminiMetadataAccounts{accounts: accounts}
	if groupID != nil {
		snapshot.groupID, snapshot.hasGroupID = *groupID, true
	}
	ctx = context.WithValue(ctx, geminiMetadataAccountsKey{}, snapshot)
	hasGemini := false
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformGemini {
			continue
		}
		hasGemini = true
		if len(account.GetModelMapping()) == 0 {
			return ctx, nil, nil
		}
	}
	if !hasGemini {
		return ctx, nil, nil
	}
	seen := make(map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformGemini &&
			(account.Platform != PlatformAntigravity || !account.IsMixedSchedulingEnabled()) {
			continue
		}
		for id, target := range account.GetModelMapping() {
			if account.Platform == PlatformAntigravity &&
				!strings.HasPrefix(strings.ToLower(id), "gemini-") &&
				!strings.HasPrefix(strings.ToLower(target), "gemini-") {
				continue
			}
			if strings.TrimSpace(id) != "" {
				seen[id] = struct{}{}
			}
		}
	}
	models := make([]string, 0, len(seen))
	for id := range seen {
		models = append(models, id)
	}
	sort.Strings(models)
	return ctx, models, nil
}

func geminiMetadataAccountsFromContext(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool) {
	snapshot, _ := ctx.Value(geminiMetadataAccountsKey{}).(*geminiMetadataAccounts)
	if snapshot == nil || snapshot.hasGroupID != (groupID != nil) ||
		(groupID != nil && snapshot.groupID != *groupID) {
		return nil, false
	}
	if platform != PlatformGemini && platform != PlatformAntigravity {
		return nil, false
	}
	accounts := make([]Account, 0, len(snapshot.accounts))
	for _, account := range snapshot.accounts {
		if account.Platform == platform || (platform == PlatformGemini && !hasForcePlatform && account.Platform == PlatformAntigravity) {
			accounts = append(accounts, account)
		}
	}
	return accounts, true
}

// DeclaredGeminiModelsCover uses the same trailing-wildcard and alias
// normalization rules as account mapping lookup.
func DeclaredGeminiModelsCover(models []string, requested string) bool {
	normalized := normalizeRequestedModelForLookup(PlatformGemini, requested)
	for _, model := range models {
		if matchWildcard(model, requested) || matchWildcard(model, normalized) {
			return true
		}
	}
	return false
}
