package service

import "strings"

func AccountTypeSupportsQuota(accountType string) bool {
	switch strings.ToLower(accountType) {
	case AccountTypeAPIKey, AccountTypeBedrock, AccountTypeServiceAccount:
		return true
	}
	return false
}

func (a *Account) SupportsAccountQuota() bool {
	return AccountTypeSupportsQuota(a.Type)
}
