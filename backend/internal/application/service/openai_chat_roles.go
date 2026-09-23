package service

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// requiresSystemChatRole identifies native Chat Completions destinations that
// reject the newer developer role. The decision is made after account and
// destination selection, so a retry on another account can receive the
// original request unchanged.
func requiresSystemChatRole(account *Account, targetURL string) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	switch account.Platform {
	case PlatformDeepseek, PlatformKimi, PlatformZhipu:
		return true
	}
	u, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "api.deepseek.com", "api.kimi.com", "api.moonshot.cn", "api.moonshot.ai", "open.bigmodel.cn", "api.z.ai":
		return true
	default:
		return false
	}
}

// normalizeStrictChatDeveloperRoles adapts only the role field for strict Chat
// upstreams. It deliberately avoids reordering or merging messages and uses
// RawMessage values so unknown fields and large numeric metadata survive the
// per-account adaptation.
func normalizeStrictChatDeveloperRoles(account *Account, targetURL string, body []byte) ([]byte, error) {
	if !requiresSystemChatRole(account, targetURL) {
		return body, nil
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return nil, errors.New("chat role normalization: invalid request object")
	}
	raw, exists := root["messages"]
	if !exists {
		return body, nil
	}
	var messages []json.RawMessage
	if json.Unmarshal(raw, &messages) != nil {
		return nil, errors.New("chat role normalization: invalid messages array")
	}
	changed := false
	for i, rawMessage := range messages {
		var message map[string]json.RawMessage
		if json.Unmarshal(rawMessage, &message) != nil || message == nil {
			return nil, errors.New("chat role normalization: invalid message object")
		}
		rawRole, ok := message["role"]
		if !ok {
			return nil, errors.New("chat role normalization: invalid message role")
		}
		var role string
		if json.Unmarshal(rawRole, &role) != nil {
			return nil, errors.New("chat role normalization: invalid message role")
		}
		if role != "developer" {
			continue
		}
		message["role"] = json.RawMessage(`"system"`)
		updated, err := json.Marshal(message)
		if err != nil {
			return nil, errors.New("chat role normalization: cannot encode message")
		}
		messages[i] = updated
		changed = true
	}
	if !changed {
		return body, nil
	}
	updatedMessages, err := json.Marshal(messages)
	if err != nil {
		return nil, errors.New("chat role normalization: cannot encode messages")
	}
	root["messages"] = updatedMessages
	updated, err := json.Marshal(root)
	if err != nil {
		return nil, errors.New("chat role normalization: cannot encode request")
	}
	return updated, nil
}
