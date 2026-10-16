package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
)

type openAIProofIdentityContextKey struct{}

// Written by the outbound identity builder, including full metadata-only
// installation projection. Inbound installation headers are not authoritative.
type openAIProofIdentity struct {
	accountID      int64
	accountKey     string
	installationID string
	threadID       string
}

func newOpenAIProofIdentity(account *Account, ids *codexFingerprintIDs) openAIProofIdentity {
	if account == nil || !account.IsOpenAIOAuth() {
		return openAIProofIdentity{}
	}
	identity := openAIProofIdentity{accountID: account.ID, accountKey: account.CodexVirtualClientKey(), installationID: resolveConvergedInstallationID(account)}
	if ids != nil && ids.mode != codexFingerprintOff && ids.installationID != "" {
		identity.installationID, identity.threadID = ids.installationID, ids.threadID
	}
	return identity
}

func newOpenAIProofIdentityFromHeaders(account *Account, ids *codexFingerprintIDs, headers http.Header) openAIProofIdentity {
	identity := newOpenAIProofIdentity(account, ids)
	if identity.threadID == "" && headers != nil {
		identity.threadID = strings.TrimSpace(headers.Get("thread-id"))
	}
	return identity
}

func withOpenAIProofIdentity(ctx context.Context, identity openAIProofIdentity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIProofIdentityContextKey{}, identity)
}

func openAIProofKey(account, owner *Account, origin, installationID string) string {
	if account == nil || owner == nil || !owner.IsOpenAIOAuth() || origin == "" || installationID == "" {
		return ""
	}
	principal := strings.TrimSpace(owner.GetChatGPTAccountID())
	if principal == "" {
		// A duplicated device ID must not merge unknown credential owners.
		principal = "local:" + strconv.FormatInt(owner.ID, 10)
	} else {
		principal = "chatgpt:" + principal
	}
	user := strings.TrimSpace(owner.GetCredential("chatgpt_user_id"))
	if user == "" {
		user = "local:" + strconv.FormatInt(owner.ID, 10)
	}
	encoded, _ := json.Marshal([]string{principal, user, installationID, origin})
	digest := sha256.Sum256(encoded)
	return "openai:integrity:v3:" + hex.EncodeToString(digest[:])
}

func openAIProofScope(ctx context.Context, account, owner *Account, origin string, headers http.Header) liveattestation.RequestScope {
	if account == nil || owner == nil || origin == "" {
		return liveattestation.RequestScope{}
	}
	identity := newOpenAIProofIdentity(account, nil)
	if ctx != nil {
		staged, ok := ctx.Value(openAIProofIdentityContextKey{}).(openAIProofIdentity)
		if ok && staged.accountID == account.ID && staged.accountKey == account.CodexVirtualClientKey() && staged.installationID != "" {
			identity = staged
		}
	}
	threadID := identity.threadID
	if len(threadID) > 256 || strings.ContainsAny(threadID, "\r\n\x00") {
		threadID = ""
	}
	return liveattestation.RequestScope{Key: openAIProofKey(account, owner, origin, identity.installationID), InstallationID: identity.installationID, ThreadID: threadID}
}
