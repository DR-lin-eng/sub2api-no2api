package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AccountQualityDegradedAccount is the credential-safe projection used by the
// admin degraded-account export. It intentionally contains only the local
// account ID, the provider email and the reason that made the account visible.
type AccountQualityDegradedAccount struct {
	AccountID     int64      `json:"account_id"`
	Email         string     `json:"email"`
	QualityStatus string     `json:"quality_status"`
	Reason        string     `json:"reason"`
	HTTPStatus    *int       `json:"http_status,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
}

type AccountQualityDegradedAccountsResponse struct {
	Items []AccountQualityDegradedAccount `json:"items"`
	Total int                             `json:"total"`
}

var qualityHTTP401Pattern = regexp.MustCompile(`(?i)(^|[^0-9])401([^0-9]|$)`)

// ListDegradedQualityAccounts returns current OAuth quality-degraded accounts
// plus OAuth accounts whose latest recorded quality/upstream error is HTTP 401.
// API-key and service-account records are excluded even if legacy metadata
// contains a degraded marker.
func (s *AccountQualityMonitoringService) ListDegradedQualityAccounts(ctx context.Context) (*AccountQualityDegradedAccountsResponse, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrAccountInspectionUnavailable
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", AccountTypeOAuth, "", "", 0, "")
	if err != nil {
		return nil, fmt.Errorf("list degraded quality accounts: %w", err)
	}
	state, err := s.loadState(ctx)
	if err != nil {
		return nil, fmt.Errorf("load quality state for degraded accounts: %w", err)
	}
	stateByID := make(map[int64]AccountInspectionAccountResult, len(state.Results))
	for _, result := range state.Results {
		stateByID[result.AccountID] = result
	}
	items := make([]AccountQualityDegradedAccount, 0)
	for _, account := range accounts {
		if account.Type != AccountTypeOAuth || (account.Platform != PlatformOpenAI && account.Platform != PlatformGemini) {
			continue
		}
		qualityStatus := qualityAccountStatus(account.Extra)
		unauthorizedMessage, unauthorized := qualityAccount401Error(account)
		if stateResult, ok := stateByID[account.ID]; ok {
			if stateResult.QualityStatus == "degraded" || qualityStatus == "" {
				qualityStatus = stateResult.QualityStatus
			}
			if !unauthorized && qualityContains401(stateResult.QualityError) {
				unauthorizedMessage, unauthorized = stateResult.QualityError, true
			}
		}
		if qualityStatus != "degraded" && !unauthorized {
			continue
		}
		reason := "degraded"
		status := qualityStatus
		if unauthorized {
			reason = "unauthorized"
			if status == "" || status == "healthy" {
				status = "error"
			}
		}
		if status == "" {
			status = "degraded"
		}
		entry := AccountQualityDegradedAccount{AccountID: account.ID, Email: accountQualityEmail(account), QualityStatus: status, Reason: reason}
		if unauthorized {
			code := 401
			entry.HTTPStatus, entry.ErrorMessage = &code, quality401Summary(unauthorizedMessage)
		}
		if checkedAt := qualityAccountLastCheckedAt(account.Extra); checkedAt != nil {
			entry.LastCheckedAt = checkedAt
		} else if stateResult, ok := stateByID[account.ID]; ok {
			checkedAt := stateResult.QualityCompletedAt
			if checkedAt == nil {
				checkedAt = stateResult.QualityStartedAt
			}
			entry.LastCheckedAt = checkedAt
		}
		items = append(items, entry)
	}
	return &AccountQualityDegradedAccountsResponse{Items: items, Total: len(items)}, nil
}

func qualityAccountStatus(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	status, _ := extra[accountQualityStatusExtraKey].(string)
	return strings.ToLower(strings.TrimSpace(status))
}

func accountQualityEmail(account Account) string {
	for _, key := range []string{"email", "email_address", "account_email"} {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	for _, key := range []string{"user", "account", "profile"} {
		if nested, ok := account.Credentials[key].(map[string]any); ok {
			for _, emailKey := range []string{"email", "email_address"} {
				if value, ok := nested[emailKey].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

func qualityAccount401Error(account Account) (string, bool) {
	if message := strings.TrimSpace(account.ErrorMessage); message != "" && qualityContains401(message) {
		return message, true
	}
	if account.Extra == nil {
		return "", false
	}
	if message, ok := qualityFind401(account.Extra[accountQualityHistoryExtraKey], 0); ok {
		return message, true
	}
	return "", false
}

func qualityContains401(value string) bool {
	lower := strings.ToLower(value)
	return qualityHTTP401Pattern.MatchString(value) || strings.Contains(lower, "unauthorized")
}

func quality401Summary(value string) string {
	value = strings.TrimSpace(value)
	if colon := strings.IndexByte(value, ':'); colon > 0 {
		value = strings.TrimSpace(value[:colon])
	}
	if !qualityContains401(value) {
		return "HTTP 401 Unauthorized"
	}
	return truncateQualityError(value)
}

func qualityFind401(value any, depth int) (string, bool) {
	if depth > 4 || value == nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		if qualityContains401(typed) {
			return typed, true
		}
	case []any:
		for _, item := range typed {
			if message, ok := qualityFind401(item, depth+1); ok {
				return message, true
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if message, ok := qualityFind401(item, depth+1); ok {
				return message, true
			}
		}
	case map[string]any:
		if message, ok := typed["error"].(string); ok && qualityContains401(message) {
			return message, true
		}
		for key, item := range typed {
			if key == "error" {
				continue
			}
			if message, ok := qualityFind401(item, depth+1); ok {
				return message, true
			}
		}
	}
	return "", false
}

func qualityAccountLastCheckedAt(extra map[string]any) *time.Time {
	if extra == nil {
		return nil
	}
	raw, _ := extra[accountQualityLastCheckedExtraKey].(string)
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	value = value.UTC()
	return &value
}
