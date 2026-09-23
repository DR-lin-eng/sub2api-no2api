package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	openAICodexTurnStateTokenTTL             = time.Hour
	openAICodexTurnStateMaxBytes             = 8 << 10
	openAICodexTurnStateRotationMessage      = "encrypted_content_rotation"
	openAICodexEncryptedContentNormalDelta   = 0
	openAICodexEncryptedContentPlus16Delta   = 16
	openAICodexObservationMaxPayloadBytes    = 256 << 10
	openAICodexObservationMaxJSONNodes       = 256
	openAICodexObservationMaxJSONDepth       = 32
	openAICodexRotationObservedContextPrefix = "codex_encrypted_content_rotation_observed:"
)

type CodexTurnStateTokenMetadata struct {
	Valid              bool       `json:"valid"`
	Expired            bool       `json:"expired"`
	Version            int        `json:"version"`
	VersionHex         string     `json:"version_hex,omitempty"`
	TokenCharacters    int        `json:"token_characters"`
	TokenBytes         int        `json:"token_bytes"`
	TokenBytesKnown    bool       `json:"token_bytes_known"`
	IssuedAt           *time.Time `json:"issued_at,omitempty"`
	EstimatedExpiresAt *time.Time `json:"estimated_expires_at,omitempty"`
	ParseError         string     `json:"parse_error,omitempty"`
}

type CodexEncryptedContentObservation struct {
	LastBytes      int        `json:"last_bytes"`
	LastBytesKnown bool       `json:"last_bytes_known"`
	BaselineBytes  int        `json:"baseline_bytes"`
	DeltaBytes     int        `json:"delta_bytes"`
	Classification string     `json:"classification"`
	LastObservedAt *time.Time `json:"last_observed_at,omitempty"`
}

type CodexTurnStateRotationObservation struct {
	Count      uint64     `json:"count"`
	LastAt     *time.Time `json:"last_at,omitempty"`
	LastReason string     `json:"last_reason,omitempty"`
}

type CodexTurnStateProbeObservation struct {
	MissingSince  *time.Time `json:"missing_since,omitempty"`
	LastHealthyAt *time.Time `json:"last_healthy_at,omitempty"`
	NextProbeAt   *time.Time `json:"next_probe_at,omitempty"`
	InFlight      bool       `json:"in_flight"`
	Recovering    bool       `json:"recovering"`
}

type CodexTurnStateObservation struct {
	AccountID            int64                             `json:"account_id"`
	Model                string                            `json:"model"`
	Source               string                            `json:"source"`
	ProxyEnabled         bool                              `json:"proxy_enabled"`
	LastSeenAt           *time.Time                        `json:"last_seen_at,omitempty"`
	LastResponseAt       *time.Time                        `json:"last_response_at,omitempty"`
	LastResponseHadState bool                              `json:"last_response_had_state"`
	LastAcceptedAt       *time.Time                        `json:"last_accepted_at,omitempty"`
	StateDigest          string                            `json:"state_digest,omitempty"`
	LengthMatch          bool                              `json:"length_match"`
	State                CodexTurnStateTokenMetadata       `json:"state"`
	EncryptedContent     CodexEncryptedContentObservation  `json:"encrypted_content"`
	Rotation             CodexTurnStateRotationObservation `json:"rotation"`
	Probe                CodexTurnStateProbeObservation    `json:"probe"`
}

type CodexTurnStateObservabilitySnapshot struct {
	GeneratedAt     time.Time                   `json:"generated_at"`
	Scope           string                      `json:"scope"`
	Enabled         bool                        `json:"enabled"`
	TargetLength    int                         `json:"target_length"`
	TokenTTLSeconds int                         `json:"token_ttl_seconds"`
	Items           []CodexTurnStateObservation `json:"items"`
}

type codexTurnStateObservation struct {
	AccountID            int64
	Model                string
	Source               string
	ProxyEnabled         bool
	LastSeenAt           time.Time
	LastResponseAt       time.Time
	LastResponseHadState bool
	LastAcceptedAt       time.Time
	State                CodexTurnStateTokenMetadata
	EncryptedContent     CodexEncryptedContentObservation
	Rotation             CodexTurnStateRotationObservation
	StateDigest          string
	LengthMatch          bool
}

func parseOpenAICodexTurnState(state string, now time.Time) CodexTurnStateTokenMetadata {
	state = strings.TrimSpace(state)
	metadata := CodexTurnStateTokenMetadata{TokenCharacters: utf8.RuneCountInString(state)}
	if state == "" {
		metadata.ParseError = "empty"
		return metadata
	}
	if len(state) > openAICodexTurnStateMaxBytes {
		metadata.ParseError = "too_large"
		return metadata
	}
	raw, err := decodeOpenAICodexTurnState(state)
	if err != nil {
		metadata.ParseError = "invalid_base64"
		return metadata
	}
	metadata.TokenBytes = len(raw)
	metadata.TokenBytesKnown = true
	if len(raw) < 9 {
		metadata.ParseError = "too_short"
		return metadata
	}
	metadata.Version = int(raw[0])
	metadata.VersionHex = fmt.Sprintf("0x%02x", raw[0])
	if raw[0] != 0x80 {
		metadata.ParseError = "unsupported_version"
		return metadata
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	seconds := binary.BigEndian.Uint64(raw[1:9])
	maxFuture := now.Unix()
	if maxFuture < 0 || seconds == 0 || seconds > uint64(maxFuture)+uint64(10*365*24*60*60) {
		metadata.ParseError = "invalid_timestamp"
		return metadata
	}
	issued := time.Unix(int64(seconds), 0).UTC()
	expires := issued.Add(openAICodexTurnStateTokenTTL)
	metadata.IssuedAt = &issued
	metadata.EstimatedExpiresAt = &expires
	metadata.Valid = true
	metadata.Expired = !now.Before(expires)
	return metadata
}

func decodeOpenAICodexTurnState(state string) ([]byte, error) {
	state = strings.TrimSpace(state)
	decoders := []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding}
	var lastErr error
	for _, decoder := range decoders {
		raw, err := decoder.DecodeString(state)
		if err == nil {
			return raw, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *OpenAIGatewayService) observeCodexTurnStateMetadata(ctx context.Context, c *gin.Context, account *Account, model, state, source string) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || !s.codexAutoTurnStateModelIsWatched(ctx, model) {
		return
	}
	key := openAICodexAutoTurnStateKey(account, model)
	if key == "" {
		return
	}
	now := time.Now().UTC()
	metadata := parseOpenAICodexTurnState(state, now)
	s.codexTurnStateObservabilityMu.Lock()
	if s.codexTurnStateObservability == nil {
		s.codexTurnStateObservability = make(map[string]codexTurnStateObservation, 64)
	}
	item := s.codexTurnStateObservability[key]
	item.AccountID = account.ID
	item.Model = strings.ToLower(strings.TrimSpace(model))
	item.Source = strings.TrimSpace(source)
	item.ProxyEnabled = account.ProxyID != nil && account.Proxy != nil
	item.LastSeenAt = now
	item.LastResponseAt = now
	item.LastResponseHadState = state != ""
	if state != "" {
		item.State = metadata
		item.LengthMatch = metadata.TokenCharacters == s.codexAutoTurnStateTargetLength(ctx)
		digest := sha256.Sum256([]byte(state))
		item.StateDigest = fmt.Sprintf("sha256:%x", digest[:6])
	}
	if state != "" && item.LengthMatch && (!metadata.Valid || !metadata.Expired) {
		item.LastAcceptedAt = now
	}
	s.codexTurnStateObservability[key] = item
	if len(s.codexTurnStateObservability) > openAICodexAutoTurnStateMaxEntries {
		for candidate := range s.codexTurnStateObservability {
			delete(s.codexTurnStateObservability, candidate)
			break
		}
	}
	s.codexTurnStateObservabilityMu.Unlock()
}

func (s *OpenAIGatewayService) observeCodexEncryptedContentPayload(ctx context.Context, c *gin.Context, account *Account, model string, payload []byte, source string) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || len(payload) == 0 || !s.codexAutoTurnStateModelIsWatched(ctx, model) {
		return
	}
	if len(payload) > openAICodexObservationMaxPayloadBytes {
		return
	}
	hasCipher := bytes.Contains(payload, []byte(`"encrypted_content"`))
	hasRotation := bytes.Contains(payload, []byte("invalid_encrypted_content"))
	if !hasRotation && (bytes.Contains(payload, []byte("encrypted")) || bytes.Contains(payload, []byte("Encrypted"))) {
		hasRotation = bytes.Contains(bytes.ToLower(payload), []byte("encrypted content could not"))
	}
	if !hasCipher && !hasRotation {
		return
	}
	if !json.Valid(payload) {
		if hasRotation {
			s.recordCodexEncryptedContentRotationError(c, account, model, source)
		}
		return
	}
	var value any
	if json.Unmarshal(payload, &value) != nil {
		return
	}
	lengths, rotation := inspectCodexEncryptedContentJSON(value)
	if rotation || hasRotation {
		s.recordCodexEncryptedContentRotationError(c, account, model, source)
	}
	for _, length := range lengths {
		s.recordCodexEncryptedContentLength(account, model, length, source)
	}
}

func inspectCodexEncryptedContentJSON(value any) ([]int, bool) {
	lengths := make([]int, 0, 2)
	nodes := 0
	rotation := false
	var walk func(any, int)
	walk = func(node any, depth int) {
		if nodes >= openAICodexObservationMaxJSONNodes || depth > openAICodexObservationMaxJSONDepth || len(lengths) >= 16 {
			return
		}
		nodes++
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if nodes >= openAICodexObservationMaxJSONNodes || len(lengths) >= 16 {
					break
				}
				if key == "encrypted_content" {
					if text, ok := child.(string); ok && text != "" {
						decoded, err := decodeOpenAICodexEncryptedContent(text)
						if err == nil && len(decoded) > 0 {
							lengths = append(lengths, len(decoded))
						}
					}
					continue
				}
				if text, ok := child.(string); ok {
					switch strings.ToLower(key) {
					case "code", "type":
						rotation = rotation || strings.EqualFold(strings.TrimSpace(text), "invalid_encrypted_content")
					case "message":
						rotation = rotation || strings.Contains(strings.ToLower(text), "encrypted content could not")
					}
				}
				walk(child, depth+1)
			}
		case []any:
			for _, child := range typed {
				if nodes >= openAICodexObservationMaxJSONNodes || len(lengths) >= 16 {
					break
				}
				walk(child, depth+1)
			}
		}
	}
	walk(value, 0)
	return lengths, rotation
}

func decodeOpenAICodexEncryptedContent(value string) ([]byte, error) {
	var lastErr error
	for _, decoder := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		decoded, err := decoder.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *OpenAIGatewayService) recordCodexEncryptedContentLength(account *Account, model string, length int, source string) {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" || length <= 0 {
		return
	}
	now := time.Now().UTC()
	s.codexTurnStateObservabilityMu.Lock()
	if s.codexTurnStateObservability == nil {
		s.codexTurnStateObservability = make(map[string]codexTurnStateObservation, 64)
	}
	item := s.codexTurnStateObservability[key]
	item.AccountID = account.ID
	item.Model = strings.ToLower(strings.TrimSpace(model))
	item.ProxyEnabled = account.ProxyID != nil && account.Proxy != nil
	if length > 0 && (item.EncryptedContent.BaselineBytes == 0 || length < item.EncryptedContent.BaselineBytes) {
		item.EncryptedContent.BaselineBytes = length
	}
	item.EncryptedContent.LastBytes = length
	item.EncryptedContent.LastBytesKnown = true
	item.EncryptedContent.DeltaBytes = length - item.EncryptedContent.BaselineBytes
	switch {
	case length == 0 || item.EncryptedContent.BaselineBytes == 0:
		item.EncryptedContent.Classification = "unknown"
	case item.EncryptedContent.DeltaBytes == openAICodexEncryptedContentNormalDelta:
		item.EncryptedContent.Classification = "baseline"
	case item.EncryptedContent.DeltaBytes == openAICodexEncryptedContentPlus16Delta:
		item.EncryptedContent.Classification = "plus_16_hint"
	default:
		item.EncryptedContent.Classification = "other"
	}
	item.EncryptedContent.LastObservedAt = &now
	item.Source = strings.TrimSpace(source)
	item.LastSeenAt = now
	s.codexTurnStateObservability[key] = item
	s.codexTurnStateObservabilityMu.Unlock()
}

func (s *OpenAIGatewayService) recordCodexEncryptedContentRotationError(c *gin.Context, account *Account, model, source string) {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" {
		return
	}
	now := time.Now().UTC()
	s.codexTurnStateObservabilityMu.Lock()
	if s.codexTurnStateObservability == nil {
		s.codexTurnStateObservability = make(map[string]codexTurnStateObservation, 64)
	}
	item := s.codexTurnStateObservability[key]
	if c != nil {
		contextKey := openAICodexRotationObservedContextPrefix + key
		if _, observed := c.Get(contextKey); observed {
			s.codexTurnStateObservabilityMu.Unlock()
			return
		}
		c.Set(contextKey, struct{}{})
	}
	item.AccountID = account.ID
	item.Model = strings.ToLower(strings.TrimSpace(model))
	item.Rotation.Count++
	item.Rotation.LastAt = &now
	item.Rotation.LastReason = openAICodexTurnStateRotationMessage
	item.Source = strings.TrimSpace(source)
	item.LastSeenAt = now
	s.codexTurnStateObservability[key] = item
	s.codexTurnStateObservabilityMu.Unlock()
}

func (s *OpenAIGatewayService) CodexTurnStateObservability(ctx context.Context) CodexTurnStateObservabilitySnapshot {
	now := time.Now().UTC()
	snapshot := CodexTurnStateObservabilitySnapshot{
		GeneratedAt:     now,
		Scope:           "current_node",
		TokenTTLSeconds: int(openAICodexTurnStateTokenTTL / time.Second),
		Items:           make([]CodexTurnStateObservation, 0),
	}
	if s == nil {
		return snapshot
	}
	if s.settingService != nil {
		settings := s.settingService.CodexSimulationSettingsSnapshot(ctx)
		snapshot.Enabled = settings.TurnStateAutoReplayEnabled
		snapshot.TargetLength = settings.TurnStateTargetLength
	}
	s.codexAutoProbeMu.Lock()
	targets := make([]openAICodexAutoProbeTarget, 0, len(s.codexAutoProbeTargets))
	for _, target := range s.codexAutoProbeTargets {
		targets = append(targets, target)
	}
	s.codexAutoProbeMu.Unlock()
	s.codexTurnStateObservabilityMu.RLock()
	items := make([]codexTurnStateObservation, 0, len(s.codexTurnStateObservability))
	for _, item := range s.codexTurnStateObservability {
		items = append(items, item)
	}
	s.codexTurnStateObservabilityMu.RUnlock()
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		item := &items[i]
		if item.State.EstimatedExpiresAt != nil {
			item.State.Expired = !now.Before(*item.State.EstimatedExpiresAt)
		}
		seen[openAICodexAutoTurnStateKey(&Account{ID: item.AccountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, item.Model)] = struct{}{}
	}
	for _, target := range targets {
		if _, ok := seen[target.Key]; ok {
			continue
		}
		items = append(items, codexTurnStateObservation{AccountID: target.AccountID, Model: target.Model})
	}
	for _, item := range items {
		probe := CodexTurnStateProbeObservation{}
		key := openAICodexAutoTurnStateKey(&Account{ID: item.AccountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, item.Model)
		s.codexAutoProbeMu.Lock()
		target, ok := s.codexAutoProbeTargets[key]
		s.codexAutoProbeMu.Unlock()
		if ok {
			probe.MissingSince = codexTimePtr(target.MissingSince)
			probe.LastHealthyAt = codexTimePtr(target.LastHealthyAt)
			probe.NextProbeAt = codexTimePtr(target.NextProbeAt)
			probe.InFlight = target.InFlight
			probe.Recovering = target.Recovering
		}
		snapshot.Items = append(snapshot.Items, CodexTurnStateObservation{
			AccountID:            item.AccountID,
			Model:                item.Model,
			Source:               item.Source,
			ProxyEnabled:         item.ProxyEnabled,
			LastSeenAt:           codexTimePtr(item.LastSeenAt),
			LastResponseAt:       codexTimePtr(item.LastResponseAt),
			LastResponseHadState: item.LastResponseHadState,
			LastAcceptedAt:       codexTimePtr(item.LastAcceptedAt),
			StateDigest:          item.StateDigest,
			LengthMatch:          item.LengthMatch,
			State:                item.State,
			EncryptedContent:     item.EncryptedContent,
			Rotation:             item.Rotation,
			Probe:                probe,
		})
	}
	sort.Slice(snapshot.Items, func(i, j int) bool {
		if snapshot.Items[i].AccountID == snapshot.Items[j].AccountID {
			return snapshot.Items[i].Model < snapshot.Items[j].Model
		}
		return snapshot.Items[i].AccountID < snapshot.Items[j].AccountID
	})
	return snapshot
}

func codexTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
