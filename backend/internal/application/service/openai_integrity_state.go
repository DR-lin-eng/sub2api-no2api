package service

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	openAIIntegrityStateTTL = 24 * time.Hour
	openAIIntegrityLocalMax = 4096
)

var openAIIntegrityStatePattern = regexp.MustCompile(`^ois1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

// OpenAIIntegrityStateCache is authoritative when supplied. CAS must be atomic
// across replicas; a read followed by an unconditional SET is insufficient.
type OpenAIIntegrityStateCache interface {
	GetOpenAIIntegrityState(context.Context, string) (string, bool, error)
	CompareAndSwapOpenAIIntegrityState(context.Context, string, string, string, time.Duration) (bool, error)
}

type openAIIntegrityReceiptContextKey struct{}

type openAIIntegrityReceipt struct {
	key      string
	expected string
	shared   bool
}

type openAIIntegrityBinding struct {
	value     string
	expiresAt time.Time
}

func validOpenAIIntegrityState(value string) bool {
	return len(value) > 0 && len(value) <= 2048 && value == strings.TrimSpace(value) && openAIIntegrityStatePattern.MatchString(value)
}

// Relays retain their OAuth bearer contract without inheriting device proofs.
func openAIDesktopProofOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || (u.Scheme != "https" && u.Scheme != "wss") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "status.openai.com" || strings.HasPrefix(host, "ab.") {
		return ""
	}
	if host != "openai.com" && host != "chatgpt.com" && !strings.HasSuffix(host, ".openai.com") && !strings.HasSuffix(host, ".chatgpt.com") {
		return ""
	}
	return "https://" + host
}

func openAIIntegrityKey(account *Account, owner *Account, origin string) string {
	// A principal repair, installation change or origin change starts a new
	// namespace. Refreshing the same principal's token preserves continuity.
	return openAIProofKey(account, owner, origin, resolveConvergedInstallationID(account))
}

func (s *OpenAIGatewayService) integritySharedCache() OpenAIIntegrityStateCache {
	if s == nil {
		return nil
	}
	shared, _ := s.cache.(OpenAIIntegrityStateCache)
	return shared
}

func (s *OpenAIGatewayService) loadIntegrityReceipt(ctx context.Context, key string) (openAIIntegrityReceipt, bool) {
	receipt := openAIIntegrityReceipt{key: key}
	if s == nil || key == "" {
		return receipt, false
	}
	if shared := s.integritySharedCache(); shared != nil {
		receipt.shared = true
		cacheCtx, cancel := withOpenAIWSStateStoreRedisTimeout(ctx)
		defer cancel()
		value, found, err := shared.GetOpenAIIntegrityState(cacheCtx, key)
		if err != nil || (found && !validOpenAIIntegrityState(value)) {
			return receipt, false
		}
		if found {
			receipt.expected = value
		}
		return receipt, true
	}
	s.openAIIntegrityMu.Lock()
	defer s.openAIIntegrityMu.Unlock()
	if binding, exists := s.openAIIntegrityStates[key]; exists {
		if time.Now().Before(binding.expiresAt) {
			receipt.expected = binding.value
		} else {
			delete(s.openAIIntegrityStates, key)
		}
	}
	return receipt, true
}

func (s *OpenAIGatewayService) attachIntegrityState(ctx context.Context, headers http.Header, key string) context.Context {
	deleteOpenAIHeaderEqualFold(headers, "X-OAI-IS")
	deleteOpenAIHeaderEqualFold(headers, "X-OAI-IS-Update")
	if ctx == nil {
		ctx = context.Background()
	}
	// Clear the receipt inherited from a previous account/transport attempt.
	ctx = context.WithValue(ctx, openAIIntegrityReceiptContextKey{}, openAIIntegrityReceipt{})
	receipt, ready := s.loadIntegrityReceipt(ctx, key)
	if !ready {
		return ctx
	}
	if receipt.expected != "" {
		headers.Set("X-OAI-IS", receipt.expected)
	}
	return context.WithValue(ctx, openAIIntegrityReceiptContextKey{}, receipt)
}

func (s *OpenAIGatewayService) captureIntegrityState(ctx context.Context, headers http.Header) bool {
	if s == nil || ctx == nil || headers == nil {
		return false
	}
	receipt, ok := ctx.Value(openAIIntegrityReceiptContextKey{}).(openAIIntegrityReceipt)
	if !ok || receipt.key == "" {
		return false
	}
	update := headers.Get("X-OAI-IS-Update")
	if len(headers.Values("X-OAI-IS-Update")) != 1 || !validOpenAIIntegrityState(update) {
		return false
	}
	if receipt.shared {
		shared := s.integritySharedCache()
		if shared == nil {
			return false
		}
		cacheCtx, cancel := withOpenAIWSStateStoreRedisTimeout(ctx)
		defer cancel()
		changed, err := shared.CompareAndSwapOpenAIIntegrityState(cacheCtx, receipt.key, receipt.expected, update, openAIIntegrityStateTTL)
		return err == nil && changed
	}
	s.openAIIntegrityMu.Lock()
	defer s.openAIIntegrityMu.Unlock()
	if s.openAIIntegrityStates == nil {
		s.openAIIntegrityStates = make(map[string]openAIIntegrityBinding)
	}
	now := time.Now()
	binding := s.openAIIntegrityStates[receipt.key]
	current := ""
	if now.Before(binding.expiresAt) {
		current = binding.value
	}
	if current != receipt.expected {
		return false
	}
	if len(s.openAIIntegrityStates) >= openAIIntegrityLocalMax {
		for key, value := range s.openAIIntegrityStates {
			if now.After(value.expiresAt) {
				delete(s.openAIIntegrityStates, key)
			}
		}
		if _, exists := s.openAIIntegrityStates[receipt.key]; !exists && len(s.openAIIntegrityStates) >= openAIIntegrityLocalMax {
			return false
		}
	}
	s.openAIIntegrityStates[receipt.key] = openAIIntegrityBinding{value: update, expiresAt: now.Add(openAIIntegrityStateTTL)}
	return true
}
