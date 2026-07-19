package service

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type RedeemCode struct {
	ID        int64
	Code      string
	Type      string
	Value     float64
	Status    string
	MaxUses   int
	UsedCount int
	// MaxUsesPerUser limits successful uses by one user; 0 means unlimited.
	MaxUsesPerUser int
	// LimitsConfigured distinguishes explicit zero (unlimited) from legacy
	// zero-value service objects created before multi-use support.
	LimitsConfigured bool
	UsedBy           *int64
	UsedAt           *time.Time
	Notes            string
	CreatedAt        time.Time
	ExpiresAt        *time.Time

	GroupID      *int64
	ValidityDays int

	User  *User
	Group *Group
}

func (r *RedeemCode) IsUsed() bool {
	return r.Status == StatusUsed || (r.MaxUses > 0 && r.UsedCount >= r.MaxUses)
}

func (r *RedeemCode) IsExpired() bool {
	return r.IsExpiredAt(time.Now())
}

func (r *RedeemCode) IsExpiredAt(now time.Time) bool {
	if r == nil {
		return false
	}
	if r.Status == StatusExpired {
		return true
	}
	return r.Status == StatusUnused && r.ExpiresAt != nil && !r.ExpiresAt.After(now)
}

func (r *RedeemCode) CanUse() bool {
	return r.Status == StatusUnused && !r.IsExpired() && (r.MaxUses <= 0 || r.UsedCount < r.MaxUses)
}

func GenerateRedeemCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
