package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/golang-jwt/jwt/v5"
)

const (
	embeddedCapabilityIssuer     = "sub2api:embedded-capability"
	embeddedCapabilityTokenUse   = "embedded_permission"
	embeddedCapabilityPermission = "custom_menu:access"
	embeddedCapabilityTTL        = 90 * time.Second
	embeddedCapabilityKeyContext = "sub2api/embedded-capability/signing-key/v1"
)

var (
	ErrEmbeddedCapabilityDenied = infraerrors.Forbidden(
		"EMBEDDED_CAPABILITY_DENIED",
		"embedded capability is not enabled for this menu",
	)
	ErrEmbeddedCapabilityInvalid = infraerrors.Unauthorized(
		"EMBEDDED_CAPABILITY_INVALID",
		"embedded capability is invalid",
	)
)

type EmbeddedCapabilityTarget struct {
	MenuID string
	Origin string
}

type EmbeddedCapabilityClaims struct {
	UserID       int64    `json:"user_id"`
	Role         string   `json:"role"`
	TokenVersion int64    `json:"token_version"`
	TokenUse     string   `json:"token_use"`
	MenuID       string   `json:"menu_id"`
	Origin       string   `json:"origin"`
	Permissions  []string `json:"permissions"`
	jwt.RegisteredClaims
}

type EmbeddedCapabilityIssueResult struct {
	Token     string
	ExpiresAt time.Time
	Target    EmbeddedCapabilityTarget
}

type embeddedCapabilityMenuItem struct {
	ID                 string `json:"id"`
	URL                string `json:"url"`
	Visibility         string `json:"visibility"`
	ForwardAccessToken bool   `json:"forward_access_token"`
}

func normalizeEmbeddedCapabilityOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil {
		return "", ErrEmbeddedCapabilityDenied
	}
	if !isAllowedEmbeddedCapabilityURL(parsed) {
		return "", ErrEmbeddedCapabilityDenied
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", ErrEmbeddedCapabilityDenied
	}
	return normalizedHTTPOrigin(parsed), nil
}

func embeddedCapabilityOriginFromURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil {
		return "", ErrEmbeddedCapabilityDenied
	}
	if !isAllowedEmbeddedCapabilityURL(parsed) {
		return "", ErrEmbeddedCapabilityDenied
	}
	return normalizedHTTPOrigin(parsed), nil
}

func isAllowedEmbeddedCapabilityURL(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "https" {
		return true
	}
	if scheme != "http" {
		return false
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func normalizedHTTPOrigin(parsed *url.URL) string {
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host
}

func (s *SettingService) ResolveEmbeddedCapabilityTarget(
	ctx context.Context,
	menuID string,
	targetOrigin string,
	role string,
) (EmbeddedCapabilityTarget, error) {
	result := EmbeddedCapabilityTarget{}
	if s == nil || s.settingRepo == nil {
		return result, ErrEmbeddedCapabilityDenied
	}
	menuID = strings.TrimSpace(menuID)
	requestedOrigin, err := normalizeEmbeddedCapabilityOrigin(targetOrigin)
	if err != nil || menuID == "" || len(menuID) > 128 {
		return result, ErrEmbeddedCapabilityDenied
	}

	raw := s.GetCustomMenuItemsRaw(ctx)
	var items []embeddedCapabilityMenuItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return result, ErrEmbeddedCapabilityDenied
	}
	for _, item := range items {
		if strings.TrimSpace(item.ID) != menuID {
			continue
		}
		if !item.ForwardAccessToken || (item.Visibility == "admin" && role != RoleAdmin) {
			return result, ErrEmbeddedCapabilityDenied
		}
		configuredOrigin, originErr := embeddedCapabilityOriginFromURL(item.URL)
		if originErr != nil || !hmac.Equal([]byte(configuredOrigin), []byte(requestedOrigin)) {
			return result, ErrEmbeddedCapabilityDenied
		}
		return EmbeddedCapabilityTarget{MenuID: menuID, Origin: configuredOrigin}, nil
	}
	return result, ErrEmbeddedCapabilityDenied
}

func embeddedCapabilitySigningKey(secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(embeddedCapabilityKeyContext))
	return mac.Sum(nil)
}

func (s *AuthService) IssueEmbeddedCapability(
	user *User,
	target EmbeddedCapabilityTarget,
) (*EmbeddedCapabilityIssueResult, error) {
	if s == nil || s.cfg == nil || user == nil || !user.IsActive() || strings.TrimSpace(s.cfg.JWT.Secret) == "" {
		return nil, ErrEmbeddedCapabilityDenied
	}
	origin, err := normalizeEmbeddedCapabilityOrigin(target.Origin)
	if err != nil || target.MenuID == "" {
		return nil, ErrEmbeddedCapabilityDenied
	}
	now := time.Now().UTC()
	expiresAt := now.Add(embeddedCapabilityTTL)
	jti, err := randomHexString(16)
	if err != nil {
		return nil, fmt.Errorf("generate embedded capability id: %w", err)
	}
	claims := &EmbeddedCapabilityClaims{
		UserID:       user.ID,
		Role:         user.Role,
		TokenVersion: resolvedTokenVersion(user),
		TokenUse:     embeddedCapabilityTokenUse,
		MenuID:       target.MenuID,
		Origin:       origin,
		Permissions:  []string{embeddedCapabilityPermission},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    embeddedCapabilityIssuer,
			Subject:   strconv.FormatInt(user.ID, 10),
			Audience:  jwt.ClaimStrings{origin},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString(embeddedCapabilitySigningKey(s.cfg.JWT.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign embedded capability: %w", err)
	}
	return &EmbeddedCapabilityIssueResult{
		Token:     tokenString,
		ExpiresAt: expiresAt,
		Target:    EmbeddedCapabilityTarget{MenuID: target.MenuID, Origin: origin},
	}, nil
}

func (s *AuthService) VerifyEmbeddedCapability(
	ctx context.Context,
	tokenString string,
	audience string,
) (*EmbeddedCapabilityClaims, error) {
	if s == nil || s.cfg == nil || s.userRepo == nil || s.settingService == nil ||
		len(tokenString) == 0 || len(tokenString) > maxTokenLength {
		return nil, ErrEmbeddedCapabilityInvalid
	}
	origin, err := normalizeEmbeddedCapabilityOrigin(audience)
	if err != nil {
		return nil, ErrEmbeddedCapabilityInvalid
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithIssuer(embeddedCapabilityIssuer),
		jwt.WithAudience(origin),
		jwt.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(tokenString, &EmbeddedCapabilityClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrEmbeddedCapabilityInvalid
		}
		return embeddedCapabilitySigningKey(s.cfg.JWT.Secret), nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, ErrEmbeddedCapabilityInvalid
	}
	claims, ok := token.Claims.(*EmbeddedCapabilityClaims)
	if !ok || claims.TokenUse != embeddedCapabilityTokenUse || claims.UserID <= 0 ||
		claims.Subject != strconv.FormatInt(claims.UserID, 10) || claims.Origin != origin ||
		claims.MenuID == "" || len(claims.Permissions) != 1 || claims.Permissions[0] != embeddedCapabilityPermission {
		return nil, ErrEmbeddedCapabilityInvalid
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil || user == nil || !user.IsActive() || user.Role != claims.Role ||
		resolvedTokenVersion(user) != claims.TokenVersion {
		return nil, ErrEmbeddedCapabilityInvalid
	}
	if _, err := s.settingService.ResolveEmbeddedCapabilityTarget(ctx, claims.MenuID, origin, user.Role); err != nil {
		if errors.Is(err, ErrEmbeddedCapabilityDenied) {
			return nil, ErrEmbeddedCapabilityInvalid
		}
		return nil, err
	}
	return claims, nil
}
